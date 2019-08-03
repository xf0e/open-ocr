package ocrworker

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// This variant of the SandwichEngine calls pdfsandwich via exec
// This implementation returns either the pdf with ocr layer only
// or merged variant of pdf plus ocr layer with the ability to
// optimize the output pdf file by calling "gs" tool
type SandwichEngine struct {
}

type SandwichEngineArgs struct {
	configVars   map[string]string `json:"config_vars"`
	lang         string            `json:"lang"`
	ocrType      string            `json:"ocr_type"`
	ocrOptimize  bool              `json:"result_optimize"`
	saveFiles    bool
	t2pConverter string
}

// NewSandwichEngineArgs generates arguments for SandwichEngine which will be used to start involved tools
func NewSandwichEngineArgs(ocrRequest OcrRequest, workerConfig WorkerConfig) (*SandwichEngineArgs, error) {
	logger := zerolog.New(os.Stdout).With().
		Str("RequestID", ocrRequest.RequestID).Timestamp().Logger()

	engineArgs := &SandwichEngineArgs{}

	if ocrRequest.EngineArgs == nil {
		return engineArgs, nil
	}
	// config vars
	configVarsMapInterfaceOrig := ocrRequest.EngineArgs["config_vars"]

	if configVarsMapInterfaceOrig != nil {

		logger.Info().Str("component", "OCR_SANDWICH").Interface("configVarsMap", configVarsMapInterfaceOrig).
			Msg("got configVarsMap")

		configVarsMapInterface := configVarsMapInterfaceOrig.(map[string]interface{})

		configVarsMap := make(map[string]string)
		for k, v := range configVarsMapInterface {
			v, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("could not convert configVar into string: %v", v)
			}
			configVarsMap[k] = v
		}

		engineArgs.configVars = configVarsMap

	}

	// language
	lang := ocrRequest.EngineArgs["lang"]
	if lang != nil {
		langStr, ok := lang.(string)
		if !ok {
			return nil, fmt.Errorf("could not convert lang into string: %v", lang)
		}
		engineArgs.lang = langStr
	}

	// select from  pdf, layer 1:pdf + layer 2:ocr_pdf
	ocrType := ocrRequest.EngineArgs["ocr_type"]
	if ocrType != nil {
		ocrTypeSrt, ok := ocrType.(string)
		if !(ok) {
			return nil, fmt.Errorf("could not convert into string: %v", ocrType)
		}
		engineArgs.ocrType = ocrTypeSrt
	}

	// set optimize flag
	ocrOptimize := ocrRequest.EngineArgs["result_optimize"]
	if ocrOptimize != nil {
		ocrOptimizeFlag, ok := ocrOptimize.(bool)
		if !(ok) {
			return nil, fmt.Errorf("could not convert into boolean: %v", ocrOptimize)
		}
		engineArgs.ocrOptimize = ocrOptimizeFlag
	}
	// if true temp files won't be deleted
	engineArgs.saveFiles = workerConfig.SaveFiles
	engineArgs.t2pConverter = workerConfig.Tiff2pdfConverter

	return engineArgs, nil

}

// return a slice that can be passed to tesseract binary as command line
// args, eg, ["-c", "tessedit_char_whitelist=0123456789", "-c", "foo=bar"]
func (t SandwichEngineArgs) Export() []string {
	var result []string
	if t.lang != "" {
		result = append(result, "-lang")
		result = append(result, t.lang)
	}
	// pdfsandwich wants the quotes before -c an after the last key e.g. -tesso '"-c arg1=key1"'
	result = append(result, "-tesso", "-c textonly_pdf=1")
	if t.configVars != nil {
		for k, v := range t.configVars {
			keyValArg := fmt.Sprintf("%s=%s", k, v)
			result = append(result, keyValArg)
		}
	}

	return result
}

// ProcessRequest will process incoming OCR request by routing it through the whole process chain
func (t SandwichEngine) ProcessRequest(ocrRequest OcrRequest, workerConfig WorkerConfig) (OcrResult, error) {

	logger := zerolog.New(os.Stdout).With().
		Str("RequestID", ocrRequest.RequestID).Timestamp().Logger()

	logger.Debug().Interface("workerConfig", workerConfig).Msg("parameter list of workerConfig")

	tmpFileName, err := func() (string, error) {
		if ocrRequest.ImgBase64 != "" {
			return t.tmpFileFromImageBase64(ocrRequest.ImgBase64, ocrRequest.RequestID)
		} else if ocrRequest.ImgUrl != "" {
			return t.tmpFileFromImageURL(ocrRequest.ImgUrl, ocrRequest.RequestID)
		} else {
			return t.tmpFileFromImageBytes(ocrRequest.ImgBytes, ocrRequest.RequestID)
		}

	}()

	if err != nil {
		logger.Error().Caller().Err(err).Str("component", "OCR_SANDWICH").Msg("error getting tmpFileName")
		return OcrResult{Text: "Internal server error", Status: "error"}, err
	}

	// detect if file type is supported
	buffer, err := readFirstBytes(tmpFileName, 64)
	if err != nil {
		logger.Warn().Str("component", "OCR_SANDWICH").Err(err).
			Str("file_name", tmpFileName).
			Msg("safety check can not be completed, processing of current file will be aborted")

		return OcrResult{Text: "WARNING: the provided file format is not supported", Status: "error"}, err
	}
	uplFileType := detectFileType(buffer[:])
	if uplFileType == "UNKNOWN" {
		err := fmt.Errorf("file format not understood")
		logger.Warn().Caller().Str("component", "OCR_SANDWICH").Err(err).
			Str("file_type", uplFileType).
			Msg("only support TIFF and PDF input files")
		return OcrResult{Text: "only support TIFF and PDF input files", Status: "error"}, err
	}
	logger.Info().Str("component", "OCR_SANDWICH").Str("file_type", uplFileType)

	engineArgs, err := NewSandwichEngineArgs(ocrRequest, workerConfig)
	if err != nil {
		logger.Error().Str("component", "OCR_SANDWICH").Err(err).Caller().Msg("error getting engineArgs")
		return OcrResult{Text: "can not build arguments", Status: "error"}, err
	}
	// getting timeout for request
	configTimeOut := ocrRequest.TimeOut

	ocrResult, err := t.processImageFile(tmpFileName, uplFileType, *engineArgs, configTimeOut)

	return ocrResult, err
}

func (t SandwichEngine) tmpFileFromImageBytes(imgBytes []byte, tmpFileName string) (string, error) {

	log.Info().Str("component", "OCR_SANDWICH").Msg("Use pdfsandwich with bytes image")
	var err error
	if tmpFileName == "" {
		tmpFileName, err = createTempFileName()
		if err != nil {
			return "", err
		}
	}

	// we have to write the contents of the image url to a temp
	// file, because the leptonica lib can't seem to handle byte arrays
	err = saveBytesToFileName(imgBytes, tmpFileName)
	if err != nil {
		return "", err
	}

	return tmpFileName, nil

}

func (t SandwichEngine) tmpFileFromImageBase64(base64Image string, tmpFileName string) (string, error) {

	log.Info().Str("component", "OCR_SANDWICH").Msg("Use pdfsandwich with base 64")
	var err error
	if tmpFileName == "" {
		tmpFileName, err = createTempFileName()
		if err != nil {
			return "", err
		}
	}

	// decoding into bytes the base64 string
	decoded, decodeError := base64.StdEncoding.DecodeString(base64Image)
	if decodeError != nil {
		return "", err
	}

	err = saveBytesToFileName(decoded, tmpFileName)
	if err != nil {
		return "", err
	}

	return tmpFileName, nil

}

func (t SandwichEngine) tmpFileFromImageURL(imgURL string, tmpFileName string) (string, error) {

	log.Info().Str("component", "OCR_SANDWICH").Msg("Use pdfsandwich with url")
	var err error
	if tmpFileName == "" {
		tmpFileName, err = createTempFileName()
		if err != nil {
			return "", err
		}
	}
	// we have to write the contents of the image url to a temp
	// file, because the leptonica lib can't seem to handle byte arrays
	err = saveUrlContentToFileName(imgURL, tmpFileName)
	if err != nil {
		return "", err
	}

	return tmpFileName, nil

}

func (t SandwichEngine) buildCmdLineArgs(inputFilename string, engineArgs SandwichEngineArgs) ([]string, string) {

	// sets output file name for pdfsandwich output file
	// and builds the argument list for external program
	// since pdfsandwich can only return pdf files the will deliver work with pdf intermediates
	// for later use we may expand the implementation
	// pdfsandwich by default default expands the name of output file wich _ocr
	cflags := engineArgs.Export()
	tmpFileExtension := "_ocr.pdf"
	ocrLayerFile := inputFilename
	cmdArgs := make([]string, 0)

	ocrLayerFile = fmt.Sprintf("%s%s", ocrLayerFile, tmpFileExtension)
	cmdArgs = append(cmdArgs, cflags...)
	cmdArgs = append(cmdArgs, inputFilename, "-o", ocrLayerFile)
	log.Info().Str("component", "OCR_SANDWICH").Interface("cmdArgs", cmdArgs)

	return cmdArgs, ocrLayerFile

}

func runExternalCmd(commandToRun string, cmdArgs []string, defaultTimeOutSeconds time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeOutSeconds)
	defer cancel()

	log.Info().Str("component", "OCR_SANDWICH").
		Str("command", commandToRun).
		Interface("cmdArgs", cmdArgs).
		Msg("running external command")

	cmd := exec.CommandContext(ctx, commandToRun, cmdArgs...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("command timed out, terminated: %v", err)
		// on deadline cancellation the output doesnt matter
		return "", err
	}
	// err = "command timed out, terminated: signal: killed"
	return string(output), err
}

func (t SandwichEngine) processImageFile(inputFilename string, uplFileType string, engineArgs SandwichEngineArgs, configTimeOut uint) (OcrResult, error) {
	// if error flag is true, input files won't be deleted
	errorFlag := false
	// inputFilename is the same as RequestID
	requestID := inputFilename
	// timeTrack(start time.Time, operation string, message string, requestID string)
	defer timeTrack(time.Now(), "processing_time", "processing time", requestID)

	log.Info().Interface("engineArgs", engineArgs).Msg("Engine arguments")
	logger := zerolog.New(os.Stdout).With().
		Str("RequestID", requestID).Timestamp().Logger()

	fileToDeliver := "temp.file"
	cmdArgs := make([]string, 0)
	ocrLayerFile := ""

	logger.Info().Str("component", "OCR_SANDWICH").
		Str("file_name", inputFilename).
		Msg("input file name")

	if uplFileType == "TIFF" {
		switch engineArgs.t2pConverter {
		case "convert":
			inputFilename = convertImageToPdf(inputFilename)
		case "tiff2pdf":
			inputFilename = tiff2Pdf(inputFilename)
		}
		if inputFilename == "" {
			err := fmt.Errorf("can not convert input image to intermediate pdf")
			logger.Error().Err(err).Caller().Msg("Error exec " + engineArgs.t2pConverter)
			return OcrResult{Status: "error"}, err
		}
	}

	ocrType := strings.ToUpper(engineArgs.ocrType)

	switch ocrType {
	case "COMBINEDPDF":
		cmdArgs, ocrLayerFile = t.buildCmdLineArgs(inputFilename, engineArgs)
		output, err := runExternalCmd("pdfsandwich", cmdArgs, time.Duration(configTimeOut)*time.Second)
		if err != nil {
			errMsg := output
			if errMsg != "" {
				errMsg = fmt.Sprintf(output, err)
				err := fmt.Errorf(errMsg)
				logger.Error().Str("component", "OCR_SANDWICH").Err(err).Caller().Msg("Error exec external command")
				errorFlag = true
				return OcrResult{Status: "error"}, err
			}
			logger.Error().Str("component", "OCR_SANDWICH").Err(err).Caller().Msg("Error exec external command")
			errorFlag = true
			return OcrResult{Status: "error"}, err
		}

		tmpOutCombinedPdf := fmt.Sprintf("%s%s", inputFilename, "_comb.pdf")

		defer func() {
			logger.Info().Str("component", "OCR_SANDWICH").Str("file_name", tmpOutCombinedPdf).
				Msg("step 2: deleting file (pdftk run)")
			if err := os.Remove(tmpOutCombinedPdf); err != nil {
				logger.Warn().Err(err).Str("component", "OCR_SANDWICH")
			}
		}()

		var combinedArgs []string
		// pdftk FILE_only_TEXT-LAYER.pdf multistamp FILE_ORIGINAL_IMAGE.pdf output FILE_OUTPUT_IMAGE_AND_TEXT_LAYER.pdf
		combinedArgs = append(combinedArgs, ocrLayerFile, "multistamp", inputFilename, "output", tmpOutCombinedPdf)
		logger.Info().Str("component", "OCR_SANDWICH").Interface("combinedArgs", combinedArgs).
			Msg("Arguments for pdftk to combine pdf files")

		outPdftk, errPdftk := exec.Command("pdftk", combinedArgs...).CombinedOutput()
		if errPdftk != nil {
			logger.Error().Err(errPdftk).Caller().Str("component", "OCR_SANDWICH").
				Str("file_name", string(outPdftk)).
				Msg("Error running command")
			errorFlag = true
			return OcrResult{Status: "error"}, err
		}

		if engineArgs.ocrOptimize {
			logger.Info().Str("component", "OCR_SANDWICH").
				Msg("optimizing was requested, perform selected operation")
			var compressedArgs []string
			tmpOutCompressedPdf := inputFilename
			tmpOutCompressedPdf = fmt.Sprintf("%s%s", tmpOutCompressedPdf, "_compr.pdf")
			defer func() {
				logger.Info().Str("component", "OCR_SANDWICH").Str("file_name", tmpOutCompressedPdf).
					Msg("step 3: deleting compressed result file (gs run)")
				if err := os.Remove(tmpOutCompressedPdf); err != nil {
					logger.Warn().Str("component", "OCR_SANDWICH").Err(err)
				}
			}()

			compressedArgs = append(
				compressedArgs,
				"-sDEVICE=pdfwrite",
				"-dCompatibilityLevel=1.5",
				"-dPDFSETTINGS=/screen",
				"-dNOPAUSE",
				"-dBATCH",
				"-dQUIET",
				"-sOutputFile="+tmpOutCompressedPdf,
				tmpOutCombinedPdf,
			)
			logger.Info().Str("component", "OCR_SANDWICH").
				Str("file_name", tmpOutCompressedPdf).
				Str("file_name", tmpOutCombinedPdf).
				Interface("compressedArgs", compressedArgs).
				Msg("tmpOutCompressedPdf, tmpOutCombinedPdf, combinedArgs ")

			outQpdf, errQpdf := exec.Command("gs", compressedArgs...).CombinedOutput()
			if errQpdf != nil {
				logger.Error().Err(errQpdf).Str("component", "OCR_SANDWICH").
					Str("outQpdf", string(outQpdf)).
					Msg("Error running command")
				errorFlag = true
				return OcrResult{Status: "error"}, err
			}

			fileToDeliver = tmpOutCompressedPdf
		} else {
			fileToDeliver = tmpOutCombinedPdf
		}
	case "OCRLAYERONLY":
		cmdArgs, ocrLayerFile = t.buildCmdLineArgs(inputFilename, engineArgs)
		cmd := exec.Command("pdfsandwich", cmdArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			errMsg := fmt.Sprintf(string(output), err)
			err := fmt.Errorf(errMsg)
			logger.Error().Caller().Err(err).Str("component", "OCR_SANDWICH").
				Msg("error exec pdfsandwich")
			errorFlag = true
			return OcrResult{Status: "error"}, err
		}
		fileToDeliver = ocrLayerFile
	case "TXT":
		cmdArgs, ocrLayerFile = t.buildCmdLineArgs(inputFilename, engineArgs)
		cmd := exec.Command("pdfsandwich", cmdArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			errMsg := fmt.Sprintf(string(output), err)
			err := fmt.Errorf(errMsg)
			logger.Error().Caller().Err(err).Str("component", "OCR_SANDWICH").
				Msg("error exec pdfsandwich")
			errorFlag = true
			return OcrResult{Status: "error"}, err
		}
		logger.Info().Str("component", "OCR_SANDWICH").Msg("extracting text from ocr")
		textFile := fmt.Sprintf("%s%s", strings.TrimSuffix(ocrLayerFile, filepath.Ext(ocrLayerFile)), ".txt")
		cmdArgsPdfToText := exec.Command("pdftotext", ocrLayerFile)
		outputPdfToText, err := cmdArgsPdfToText.CombinedOutput()
		if err != nil {
			errMsg := fmt.Sprintf(string(outputPdfToText), err)
			err := fmt.Errorf(errMsg)
			logger.Error().Caller().Err(err).Str("component", "OCR_SANDWICH").
				Msg("error exec pdftotext")
			errorFlag = true
		}
		// pdftotext will create %filename%.txt
		defer func() {
			logger.Info().Str("component", "OCR_SANDWICH").Str("file_name", textFile).
				Msg("step 2: deleting file (pdftotext run)")
			if err := os.Remove(textFile); err != nil {
				logger.Warn().Err(err).Str("component", "OCR_SANDWICH")
			}
		}()

		fileToDeliver = textFile

	default:
		err := fmt.Errorf("requested format is not supported")
		logger.Error().Err(err).Caller().Str("component", "OCR_SANDWICH")
		errorFlag = true
		return OcrResult{Status: "error"}, err
	}
	// if command line argument save_files is set or any internal processing is failed the input file won't be deleted
	if !engineArgs.saveFiles || errorFlag == true {
		defer func() {
			logger.Info().Str("component", "OCR_SANDWICH").Str("file_name", ocrLayerFile).
				Msg("step 1: deleting file (pdfsandwich run)")
			if err := os.Remove(ocrLayerFile); err != nil {
				logger.Warn().Err(err).Str("component", "OCR_SANDWICH")
			}
			logger.Info().Str("component", "OCR_SANDWICH").Str("file_name", inputFilename).
				Msg("step 1: deleting file (pdfsandwich run)")
			if err := os.Remove(inputFilename); err != nil {
				logger.Warn().Err(err).Str("component", "OCR_SANDWICH")
			}
		}()
	} else {
		inputFilenamePath, _ := filepath.Abs(inputFilename)
		ocrLayerFilePath, _ := filepath.Abs(ocrLayerFile)
		logger.Info().Str("component", "OCR_SANDWICH").Str("ocrLayerFile", ocrLayerFilePath).
			Str("inputFilename", inputFilenamePath).
			Msg("Input file and ocrLayer file were not removed for debugging purposes")
	}

	logger.Info().Str("component", "OCR_SANDWICH").Str("file_name", fileToDeliver).
		Msg("resulting file")
	outBytes, err := ioutil.ReadFile(fileToDeliver)
	if err != nil {
		logger.Error().Caller().Err(err).Str("component", "OCR_SANDWICH").
			Msg("Error getting data from result file")
		return OcrResult{Status: "error"}, err
	}
	return OcrResult{
		Text:   base64.StdEncoding.EncodeToString(outBytes),
		Status: "done",
	}, nil
}
