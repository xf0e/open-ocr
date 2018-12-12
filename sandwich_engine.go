package ocrworker

import (
	"encoding/base64"
	"fmt"
	"github.com/couchbaselabs/logg"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// This variant of the SandwichEngine calls pdfsandwich via exec
// This implementation returns either the pdf with ocr layer only
// or merged variant of pdf plus ocr layer with the ability to
// optimize the output pdf file by calling "gs" tool
type SandwichEngine struct {
}

type SandwichEngineArgs struct {
	configVars  map[string]string `json:"config_vars"`
	lang        string            `json:"lang"`
	ocrType     string            `json:"ocr_type"`
	ocrOptimize bool              `json:"result_optimize"`
}

func NewSandwichEngineArgs(ocrRequest OcrRequest) (*SandwichEngineArgs, error) {

	engineArgs := &SandwichEngineArgs{}

	if ocrRequest.EngineArgs == nil {
		return engineArgs, nil
	}
	// config vars
	configVarsMapInterfaceOrig := ocrRequest.EngineArgs["config_vars"]

	if configVarsMapInterfaceOrig != nil {

		logg.LogTo("OCR_SANDWICH", "got configVarsMap: %v type: %T", configVarsMapInterfaceOrig, configVarsMapInterfaceOrig)

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

func (t SandwichEngine) ProcessRequest(ocrRequest OcrRequest) (OcrResult, error) {

	tmpFileName, err := func() (string, error) {
		if ocrRequest.ImgBase64 != "" {
			return t.tmpFileFromImageBase64(ocrRequest.ImgBase64)
		} else if ocrRequest.ImgUrl != "" {
			return t.tmpFileFromImageUrl(ocrRequest.ImgUrl)
		} else {
			return t.tmpFileFromImageBytes(ocrRequest.ImgBytes)
		}

	}()

	if err != nil {
		logg.LogTo("OCR_SANDWICH", "error getting tmpFileName")
		return OcrResult{}, err
	}

	defer func() {
		logg.LogTo("OCR_SANDWICH", "step 0: deleting input file after convert it to pdf: %s",
			tmpFileName)
		if err := os.Remove(tmpFileName); err != nil {
			logg.LogWarn("OCR_SANDWICH", err)
		}
	}()

	// detect if file type is supported
	buffer, err := readFirstBytes(tmpFileName, 64)
	if err != nil {
		logg.LogWarn("OCR_SANDWICH", "safety check can not be completed", err)
		logg.LogWarn("OCR_SANDWICH", "processing of %s will be aborted", tmpFileName)
		return OcrResult{"WARNING: the provided file format is not supported"}, err
	}
	uplFileType := detectFileType(buffer[:])
	if uplFileType == "UNKNOWN" {
		logg.LogWarn("OCR_SANDWICH", "file type is: %s. only support TIFF and PDF input files", uplFileType)
		err := fmt.Errorf("file format not understood")
		return OcrResult{"WARNING: only support TIFF and PDF input files"}, err
	}
	logg.LogTo("OCR_SANDWICH", "file type is: %s", uplFileType)

	engineArgs, err := NewSandwichEngineArgs(ocrRequest)
	if err != nil {
		logg.LogTo("OCR_SANDWICH", "error getting engineArgs")
		return OcrResult{}, err
	}

	ocrResult, err := t.processImageFile(tmpFileName, uplFileType, *engineArgs)

	return ocrResult, err
}

func (t SandwichEngine) tmpFileFromImageBytes(imgBytes []byte) (string, error) {

	logg.LogTo("OCR_SANDWICH", "Use pdfsandwich with bytes image")

	tmpFileName, err := createTempFileName()
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

func (t SandwichEngine) tmpFileFromImageBase64(base64Image string) (string, error) {

	logg.LogTo("OCR_SANDWICH", "Use pdfsandwich with base 64")

	tmpFileName, err := createTempFileName()
	if err != nil {
		return "", err
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

func (t SandwichEngine) tmpFileFromImageUrl(imgUrl string) (string, error) {

	logg.LogTo("OCR_SANDWICH", "Use pdfsandwich with url")

	tmpFileName, err := createTempFileName()
	if err != nil {
		return "", err
	}
	// we have to write the contents of the image url to a temp
	// file, because the leptonica lib can't seem to handle byte arrays
	err = saveUrlContentToFileName(imgUrl, tmpFileName)
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
	cmdArgs := []string{}

	ocrLayerFile = fmt.Sprintf("%s%s", ocrLayerFile, tmpFileExtension)
	cmdArgs = append(cmdArgs, cflags...)
	cmdArgs = append(cmdArgs, inputFilename, "-o", ocrLayerFile)
	logg.LogTo("OCR_SANDWICH", "cmdArgs for pdfsandwich: %v", cmdArgs)

	return cmdArgs, ocrLayerFile

}

func (t SandwichEngine) processImageFile(inputFilename string, uplFileType string, engineArgs SandwichEngineArgs) (OcrResult, error) {

	fileToDeliver := "temp.file"
	cmdArgs := []string{}
	ocrLayerFile := ""

	logg.LogTo("OCR_SANDWICH", "input file name: %v", inputFilename)

	if uplFileType == "TIFF" {
		inputFilename = convertImageToPdf(inputFilename)
		if inputFilename == "" {
			err := fmt.Errorf("can not convert input image to intermediate pdf")
			logg.LogTo("OCR_SANDWICH", "Error exec pdfsandwich: %v %v", err)
			return OcrResult{}, err
		}
	}

	ocrType := strings.ToUpper(engineArgs.ocrType)
	switch ocrType {
	case "COMBINEDPDF":
		cmdArgs, ocrLayerFile = t.buildCmdLineArgs(inputFilename, engineArgs)
		cmd := exec.Command("pdfsandwich", cmdArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			logg.LogTo("OCR_SANDWICH", "Error exec pdfsandwich: %v %v", err, string(output))
			return OcrResult{}, err
		}
		tmpOutCombinedPdf, err := createTempFileName()
		tmpOutCombinedPdf = fmt.Sprintf("%s%s", tmpOutCombinedPdf, "_comb.pdf")
		if err != nil {
			return OcrResult{}, err
		}
		defer func() {
			logg.LogWarn("OCR_SANDWICH", "step 2: deleting file (pdftk run): %s",
				tmpOutCombinedPdf)
			if err := os.Remove(tmpOutCombinedPdf); err != nil {
				logg.LogWarn("OCR_SANDWICH", err)
			}
		}()

		var combinedArgs []string
		// pdftk FILE_only_TEXT-LAYER.pdf multistamp FILE_ORIGINAL_IMAGE.pdf output FILE_OUTPUT_IMAGE_AND_TEXT_LAYER.pdf
		combinedArgs = append(combinedArgs, ocrLayerFile, "multistamp", inputFilename, "output", tmpOutCombinedPdf)
		logg.LogTo("OCR_SANDWICH", "combinedArgs: %v", combinedArgs)
		outPdftk, errPdftk := exec.Command("pdftk", combinedArgs...).CombinedOutput()
		if errPdftk != nil {
			logg.LogWarn("Error running command: %s.  out: %s", errPdftk, outPdftk)
			return OcrResult{}, err
		}
		logg.LogTo("OCR_TESSERACT", "output: %v", string(outPdftk))

		if engineArgs.ocrOptimize {
			logg.LogTo("OCR_SANDWICH", "%s", "optimizing was requested. perform selected operation")
			var compressedArgs []string
			tmpOutCompressedPdf, err := createTempFileName()
			if err != nil {
				return OcrResult{}, err
			}
			tmpOutCompressedPdf = fmt.Sprintf("%s%s", tmpOutCompressedPdf, "_compr.pdf")
			defer func() {
				logg.LogWarn("OCR_SANDWICH", "step 3: deleting compressed result file (gs run): %s",
					tmpOutCompressedPdf)
				if err := os.Remove(tmpOutCompressedPdf); err != nil {
					logg.LogWarn("OCR_SANDWICH", err)
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
			logg.LogTo("OCR_SANDWICH", "tmpOutCompressedPdf: %s", tmpOutCompressedPdf)
			logg.LogTo("OCR_SANDWICH", "tmpOutCombinedPdf: %s", tmpOutCombinedPdf)
			logg.LogTo("OCR_SANDWICH", "combinedArgs: %v", compressedArgs)
			outQpdf, errQpdf := exec.Command("gs", compressedArgs...).CombinedOutput()
			if errQpdf != nil {
				logg.LogWarn("Error running command: %s.  out: %s", errQpdf, outQpdf)
				return OcrResult{}, err
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
			logg.LogTo("OCR_SANDWICH", "error exec pdfsandwich: %v %v", err, string(output))
			return OcrResult{}, err
		}

		fileToDeliver = ocrLayerFile
	case "TXT":
		cmdArgs, ocrLayerFile = t.buildCmdLineArgs(inputFilename, engineArgs)
		cmd := exec.Command("pdfsandwich", cmdArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			logg.LogTo("OCR_SANDWICH", "error exec pdfsandwich: %v %v", err, string(output))
			return OcrResult{}, err
		}
		logg.LogTo("OCR_SANDWICH", "extracting text from ocr")
		textFile := fmt.Sprintf("%s%s", strings.TrimSuffix(ocrLayerFile, filepath.Ext(ocrLayerFile)), ".txt")
		cmdArgsPdfToText := exec.Command("pdftotext", ocrLayerFile)
		outputPdfToText, err := cmdArgsPdfToText.CombinedOutput()
		if err != nil {
			logg.LogTo("OCR_SANDWICH", "error exec pdftotext: %v %v", err, string(outputPdfToText))
		}
		// pdftotext will create %filename%.txt
		defer func() {
			logg.LogWarn("OCR_SANDWICH", "step 2: deleting file (pdftotext run): %s",
				textFile)
			if err := os.Remove(textFile); err != nil {
				logg.LogWarn("OCR_SANDWICH", err)
			}
		}()

		fileToDeliver = textFile

	default:
		err := fmt.Errorf("requested format is not supported")
		logg.LogTo("OCR_SANDWICH", "error: %v", err)
		return OcrResult{}, err
	}

	defer func() {
		logg.LogTo("OCR_SANDWICH", "step 1: deleting file (pdfsandwich run): %s",
			ocrLayerFile)
		if err := os.Remove(ocrLayerFile); err != nil {
			logg.LogWarn("OCR_SANDWICH", err)
		}
		logg.LogTo("OCR_SANDWICH", "step 1: deleting file (pdfsandwich run): %s",
			inputFilename)
		if err := os.Remove(inputFilename); err != nil {
			logg.LogWarn("OCR_SANDWICH", err)
		}
	}()

	logg.LogTo("OCR_SANDWICH", "outfile %s ", fileToDeliver)
	outBytes, err := ioutil.ReadFile(fileToDeliver)
	if err != nil {
		logg.LogTo("OCR_SANDWICH", "Error getting data from result file: %v", err)
		return OcrResult{}, err
	}
	return OcrResult{
		Text: string(base64.StdEncoding.EncodeToString(outBytes)),
	}, nil
}
