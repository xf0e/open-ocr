package ocrworker

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// SandwichEngine calls pdfsandwich via exec
// This implementation returns either the pdf with ocr layer only
// or merged variant of pdf plus ocr layer with the ability to
// optimize the output pdf file by calling "gs" tool
type SandwichEngine struct{}

type SandwichEngineArgs struct {
	configVars   map[string]string `json:"config_vars"`
	lang         string            `json:"lang"`
	ocrType      string            `json:"ocr_type"`
	ocrOptimize  bool              `json:"result_optimize"`
	saveFiles    bool
	t2pConverter string
	requestID    string
	component    string
}

// NewSandwichEngineArgs generates arguments for SandwichEngine which will be used to start involved tools
func NewSandwichEngineArgs(ocrRequest *OcrRequest, workerConfig *WorkerConfig) (*SandwichEngineArgs, error) {
	engineArgs := &SandwichEngineArgs{}
	engineArgs.component = "OCR_WORKER"
	engineArgs.requestID = ocrRequest.RequestID

	logger := zerolog.New(os.Stdout).With().
		Str("RequestID", engineArgs.requestID).Str("component", engineArgs.component).Timestamp().Logger()

	if ocrRequest.EngineArgs == nil {
		return engineArgs, nil
	}
	// config vars
	configVarsMapInterfaceOrig := ocrRequest.EngineArgs["config_vars"]

	if configVarsMapInterfaceOrig != nil {

		logger.Info().Interface("configVarsMap", configVarsMapInterfaceOrig).
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

// Export return a slice that can be passed to tesseract binary as command line
// args, eg, ["-c", "tessedit_char_whitelist=0123456789", "-c", "foo=bar"]
func (t *SandwichEngineArgs) Export() []string {
	var result []string
	if t.lang != "" {
		result = append(result, "-lang", t.lang)
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
func (t SandwichEngine) ProcessRequest(ocrRequest *OcrRequest, workerConfig *WorkerConfig) (OcrResult, error) {
	logger := zerolog.New(os.Stdout).With().
		Str("component", "OCR_SANDWICH").
		Str("RequestID", ocrRequest.RequestID).Timestamp().Logger()
	// copy configuration for logging purposes to prevent leaking passwords to logs
	workerConfigToLog := workerConfig
	urlToLog, err := url.Parse(workerConfigToLog.AmqpAPIURI)
	if err == nil {
		workerConfigToLog.AmqpAPIURI = StripPasswordFromUrl(urlToLog)
	}
	urlToLog, err = url.Parse(workerConfigToLog.AmqpURI)
	if err == nil {
		workerConfigToLog.AmqpURI = StripPasswordFromUrl(urlToLog)
	}

	logger.Debug().Interface("workerConfig", workerConfigToLog).Msg("worker configuration for this request")
	logger.Info().Str("DocType", ocrRequest.DocType).
		Str("ImgUrl", ocrRequest.ImgUrl).
		Str("ReplyTo", ocrRequest.ReplyTo).
		Bool("Deferred", ocrRequest.Deferred).
		Uint16("PageNumber", ocrRequest.PageNumber).
		Uint("TimeOut", ocrRequest.TimeOut).
		Int("ImgBase64Size", len(ocrRequest.ImgBase64)).
		Int("ImgBytesSize", len(ocrRequest.ImgBytes)).
		Str("UserAgent", ocrRequest.UserAgent).
		Str("ReferenceID", ocrRequest.ReferenceID).
		Msg("ocr request data")

	tmpFileName, err := func() (string, error) {
		switch {
		case ocrRequest.ImgBase64 != "":
			return t.tmpFileFromImageBase64(ocrRequest.ImgBase64, ocrRequest.RequestID)
		case ocrRequest.ImgUrl != "":
			return t.tmpFileFromImageURL(ocrRequest.ImgUrl, ocrRequest.RequestID)
		default:
			return t.tmpFileFromImageBytes(ocrRequest.ImgBytes, ocrRequest.RequestID)
		}
	}()
	if err != nil {
		logger.Error().Caller().Err(err).Msg("error getting tmpFileName")
		return OcrResult{Text: "Internal server error", Status: "error"}, err
	}

	// detect if file type is supported
	buffer, err := readFirstBytes(tmpFileName, 64)
	if err != nil {
		logger.Warn().Err(err).
			Str("file_name", tmpFileName).
			Msg("safety check can not be completed, processing of current file will be aborted")

		return OcrResult{Text: "WARNING: provided file format is not supported", Status: "error"}, err
	}
	uplFileType := detectFileType(buffer)
	if uplFileType == "UNKNOWN" {
		err := fmt.Errorf("file format not understood")
		logger.Warn().Caller().Err(err).
			Str("file_type", uplFileType).
			Msg("only support TIFF and PDF input files")
		return OcrResult{Text: "only support TIFF and PDF input files", Status: "error"}, err
	}
	logger.Info().Str("file_type", uplFileType)

	engineArgs, err := NewSandwichEngineArgs(ocrRequest, workerConfig)
	if err != nil {
		logger.Error().Err(err).Caller().Msg("error getting engineArgs")
		return OcrResult{Text: "can not build arguments", Status: "error"}, err
	}
	// getting timeout for request
	configTimeOut := ocrRequest.TimeOut

	ocrResult, err := t.processImageFile(tmpFileName, uplFileType, engineArgs, configTimeOut)

	return ocrResult, err
}

func (SandwichEngine) tmpFileFromImageBytes(imgBytes []byte, tmpFileName string) (string, error) {
	log.Info().Str("component", "OCR_SANDWICH").Msg("Use pdfsandwich with bytes image")
	var err error
	tmpFileName, err = createTempFileName(tmpFileName)
	if err != nil {
		return "", err
	}

	// we have to write the contents of the image url to a temp
	// file, because the leptonica lib can't seem to handle byte arrays
	err = saveBytesToFileName(imgBytes, tmpFileName)
	if err != nil {
		return "", err
	}

	return tmpFileName, nil
}

func (SandwichEngine) tmpFileFromImageBase64(base64Image, tmpFileName string) (string, error) {
	log.Info().Str("component", "OCR_SANDWICH").Msg("Use pdfsandwich with base 64")
	var err error
	if tmpFileName == "" {
		tmpFileName, err = createTempFileName("")
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

func (SandwichEngine) tmpFileFromImageURL(imgURL, tmpFileName string) (string, error) {
	log.Info().Str("component", "OCR_SANDWICH").Msg("Use pdfsandwich with url")
	var err error
	if tmpFileName == "" {
		tmpFileName, err = createTempFileName("")
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

func (SandwichEngine) buildCmdLineArgs(inputFilename string, engineArgs *SandwichEngineArgs) (cmdArgs []string, ocrLayerFile string) {
	// sets output file name for pdfsandwich output file
	// and builds the argument list for external program
	// since pdfsandwich can only return pdf files the will deliver work with pdf intermediates
	// for later use we may expand the implementation
	// pdfsandwich by default default expands the name of output file wich _ocr
	cflags := engineArgs.Export()
	tmpFileExtension := "_ocr.pdf"
	ocrLayerFile = inputFilename
	cmdArgs = make([]string, 0)

	ocrLayerFile = fmt.Sprintf("%s%s", ocrLayerFile, tmpFileExtension)
	cmdArgs = append(cmdArgs, cflags...)
	cmdArgs = append(cmdArgs, inputFilename, "-o", ocrLayerFile)
	log.Info().Str("component", "OCR_SANDWICH").Interface("cmdArgs", cmdArgs)

	return cmdArgs, ocrLayerFile
}

func (SandwichEngine) runExternalCmd(commandToRun string, cmdArgs []string, defaultTimeOutSeconds time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeOutSeconds)
	defer cancel()

	log.Debug().Str("component", "OCR_SANDWICH").
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

func (t SandwichEngine) processImageFile(inputFilename, uplFileType string, engineArgs *SandwichEngineArgs, configTimeOut uint) (OcrResult, error) {
	// if error flag is true, input files won't be deleted
	errorFlag := false
	filesToDelete := make([]string, 0)
	logger := zerolog.New(os.Stdout).With().
		Str("component", "OCR_SANDWICH").
		Str("RequestID", engineArgs.requestID).Timestamp().Logger()

	// if command line argument save_files is set or any internal processing is failed the input file won't be deleted
	defer func() {
		if !engineArgs.saveFiles && !errorFlag {
			for _, element := range filesToDelete {
				fileToDelete, _ := filepath.Abs(element)
				logger.Info().Str("file_name", element).
					Bool("save_files_flag", engineArgs.saveFiles).
					Bool("errorFlag", errorFlag).
					Msg("deleting file " + element)
				if err := os.Remove(fileToDelete); err != nil {
					logger.Warn().Err(err)
				}
			}
		} else {
			logger.Info().Interface("fileList", filesToDelete).
				Msg("All input files were not removed for debugging purposes due to flags or errors while processing")
		}
	}()

	// timeTrack(start time.Time, operation string, message string, requestID string)
	defer timeTrack(time.Now(), "processing_time", "processing time", engineArgs.requestID)

	logger.Info().Interface("engineArgs", engineArgs).Msg("Engine arguments")

	fileToDeliver := "temp.file"
	var cmdArgs []string

	ocrLayerFile := ""
	alternativeConverter := ""
	originalInputfileName := inputFilename

	logger.Info().Str("file_name", inputFilename).Msg("input file name")

	filesToDelete = append(filesToDelete, inputFilename)

	if uplFileType == "TIFF" {
		switch engineArgs.t2pConverter {
		case "convert":
			alternativeConverter = "tiff2pdf"
			inputFilename = convertImageToPdf(inputFilename)
		case "tiff2pdf":
			alternativeConverter = "convert"
			inputFilename = tiff2Pdf(inputFilename)
		}
		/* if the first converter fails, we will automatically try the second one.
		If the second one fails, we will break up processing and return an error to a caller */
		if inputFilename == "" {
			err := fmt.Errorf("can not convert input image to intermediate pdf, usually this is caused by a damaged input file")
			logger.Warn().Err(err).Caller().Msg("Error exec " + engineArgs.t2pConverter + " Try to switch the image converter to " + alternativeConverter)
			switch alternativeConverter {
			case "convert":
				inputFilename = convertImageToPdf(originalInputfileName)
			case "tiff2pdf":
				inputFilename = tiff2Pdf(originalInputfileName)
			}
			if inputFilename == "" {
				err := fmt.Errorf("entirely failed to convert the input image to intermediate pdf, usually this is caused by a damaged input file")
				logger.Error().Err(err).Caller().Msg("Error exec " + alternativeConverter)
				errorFlag = true
				return OcrResult{Status: "error"}, err
			}
		}
	}

	ocrType := strings.ToUpper(engineArgs.ocrType)

	extCommandTimeout := time.Duration(configTimeOut) * time.Second

	cmdArgs, ocrLayerFile = t.buildCmdLineArgs(inputFilename, engineArgs)
	logger.Info().Str("command", "pdfsandwich").Interface("cmdArgs", cmdArgs).
		Uint("command_timeout", configTimeOut).
		Msg("running external pdfsandwich command")
	output, err := t.runExternalCmd("pdfsandwich", cmdArgs, extCommandTimeout)
	if err != nil {
		errMsg := output
		if errMsg != "" {
			errMsg = fmt.Sprintf(output, err)
			err := fmt.Errorf(errMsg)
			logger.Error().Err(err).Caller().Msg("Error exec external command")
			errorFlag = true
			return OcrResult{Status: "error"}, err
		}
		logger.Error().Err(err).Caller().Msg("Error exec external command")
		errorFlag = true
		return OcrResult{Status: "error"}, err
	}

	switch ocrType {
	case "COMBINEDPDF":

		tmpOutCombinedPdf := fmt.Sprintf("%s%s", inputFilename, "_comb.pdf")
		filesToDelete = append(filesToDelete, tmpOutCombinedPdf)

		var combinedArgs []string
		// pdftk FILE_only_TEXT-LAYER.pdf multistamp FILE_ORIGINAL_IMAGE.pdf output FILE_OUTPUT_IMAGE_AND_TEXT_LAYER.pdf
		combinedArgs = append(combinedArgs, ocrLayerFile, "multistamp", inputFilename, "output", tmpOutCombinedPdf)
		logger.Info().Interface("combinedArgs", combinedArgs).
			Msg("Arguments for pdftk to combine pdf files")

		_, errPdftk := exec.Command("pdftk", combinedArgs...).CombinedOutput()
		if errPdftk != nil {
			logger.Error().Err(errPdftk).Caller().
				Str("file_name", tmpOutCombinedPdf).
				Msg("Error running command")
			errorFlag = true
			return OcrResult{Status: "error"}, err
		}

		if engineArgs.ocrOptimize {
			logger.Info().Msg("optimizing was requested, performing selected operation")
			var compressedArgs []string
			tmpOutCompressedPdf := inputFilename
			tmpOutCompressedPdf = fmt.Sprintf("%s%s", tmpOutCompressedPdf, "_compr.pdf")
			filesToDelete = append(filesToDelete, tmpOutCompressedPdf)

			compressedArgs = append(
				compressedArgs,
				"-sDEVICE=pdfwrite",
				"-dCompatibilityLevel=1.7",
				"-dPDFSETTINGS=/prepress",
				"-dNOPAUSE",
				"-dBATCH",
				"-dQUIET",
				"-sOutputFile="+tmpOutCompressedPdf,
				tmpOutCombinedPdf,
			)
			logger.Info().Str("file_name", tmpOutCompressedPdf).
				Str("file_name", tmpOutCombinedPdf).
				Interface("compressedArgs", compressedArgs).
				Msg("tmpOutCompressedPdf, tmpOutCombinedPdf, combinedArgs ")

			outQpdf, errQpdf := exec.Command("gs", compressedArgs...).CombinedOutput()
			if errQpdf != nil {
				logger.Error().Err(errQpdf).
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
		fileToDeliver = ocrLayerFile
	case "TXT":
		// pdftotext will create %filename%.txt
		logger.Info().Msg("extracting text from ocr")
		textFile := fmt.Sprintf("%s%s", strings.TrimSuffix(ocrLayerFile, filepath.Ext(ocrLayerFile)), ".txt")
		filesToDelete = append(filesToDelete, textFile)
		cmdArgsPdfToText := exec.Command("pdftotext", ocrLayerFile)
		outputPdfToText, err := cmdArgsPdfToText.CombinedOutput()
		if err != nil {
			errMsg := fmt.Sprintf(string(outputPdfToText), err)
			err := fmt.Errorf(errMsg)
			logger.Error().Caller().Err(err).Msg("error exec pdftotext")
			errorFlag = true
		}

		fileToDeliver = textFile

	default:
		err := fmt.Errorf("requested output format is not supported")
		logger.Error().Err(err).Caller()
		errorFlag = true
		return OcrResult{Status: "error"}, err
	}

	logger.Info().Str("file_name", fileToDeliver).Msg("resulting file")
	outBytes, err := os.ReadFile(fileToDeliver)
	if err != nil {
		logger.Error().Caller().Err(err).Msg("Error getting data from result file")
		return OcrResult{Status: "error"}, err
	}
	return OcrResult{
		Text:   base64.StdEncoding.EncodeToString(outBytes),
		Status: "done",
	}, nil
}
