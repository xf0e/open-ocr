package ocrworker

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/rs/zerolog/log"
)

// This variant of the TesseractEngine calls tesseract via exec
type TesseractEngine struct {
}

type TesseractEngineArgs struct {
	configVars  map[string]string `json:"config_vars"`
	pageSegMode string            `json:"psm"`
	lang        string            `json:"lang"`
	saveFiles   bool
}

func NewTesseractEngineArgs(ocrRequest *OcrRequest) (*TesseractEngineArgs, error) {

	engineArgs := &TesseractEngineArgs{}

	if ocrRequest.EngineArgs == nil {
		return engineArgs, nil
	}

	// config vars
	configVarsMapInterfaceOrig := ocrRequest.EngineArgs["config_vars"]

	if configVarsMapInterfaceOrig != nil {

		log.Info().Str("component", "OCR_TESSERACT").
			Interface("configVarsMapInterfaceOrig", configVarsMapInterfaceOrig).
			Interface("configVarsMapInterfaceOrig", configVarsMapInterfaceOrig).Msg("got configVarsMap")

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

	// page seg mode
	pageSegMode := ocrRequest.EngineArgs["psm"]
	if pageSegMode != nil {
		pageSegModeStr, ok := pageSegMode.(string)
		if !ok {
			return nil, fmt.Errorf("could not convert psm into string: %v", pageSegMode)
		}
		engineArgs.pageSegMode = pageSegModeStr
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

	return engineArgs, nil

}

// return a slice that can be passed to tesseract binary as command line
// args, eg, ["-c", "tessedit_char_whitelist=0123456789", "-c", "foo=bar"]
func (t TesseractEngineArgs) Export() []string {
	var result []string
	for k, v := range t.configVars {
		result = append(result, "-c")
		keyValArg := fmt.Sprintf("%s=%s", k, v)
		result = append(result, keyValArg)
	}
	if t.pageSegMode != "" {
		result = append(result, "--psm", t.pageSegMode)
	}
	if t.lang != "" {
		result = append(result, "-l", t.lang)
	}

	return result
}

// ProcessRequest will process incoming OCR request by routing it through the whole process chain
func (t TesseractEngine) ProcessRequest(ocrRequest *OcrRequest, workerConfig *WorkerConfig) (OcrResult, error) {

	tmpFileName, err := func() (string, error) {
		switch {
		case ocrRequest.ImgBase64 != "":
			return t.tmpFileFromImageBase64(ocrRequest.ImgBase64)
		case ocrRequest.ImgUrl != "":
			return t.tmpFileFromImageUrl(ocrRequest.ImgUrl)
		default:
			return t.tmpFileFromImageBytes(ocrRequest.ImgBytes)
		}
	}()

	if err != nil {
		log.Error().Err(err).Str("component", "OCR_TESSERACT").Msg("error getting tmpFileName")
		return OcrResult{}, err
	}

	engineArgs, err := NewTesseractEngineArgs(ocrRequest)
	if err != nil {
		log.Error().Err(err).Str("component", "OCR_TESSERACT").Caller().Msg("error getting engineArgs")
		return OcrResult{}, err
	}

	if engineArgs.saveFiles {
		defer os.Remove(tmpFileName)
	}

	ocrResult, err := t.processImageFile(tmpFileName, *engineArgs)

	return ocrResult, err

}

func (t TesseractEngine) tmpFileFromImageBytes(imgBytes []byte) (string, error) {

	log.Info().Str("component", "OCR_TESSERACT").Msg("Use tesseract with bytes image")

	tmpFileName, err := createTempFileName("")
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

func (t TesseractEngine) tmpFileFromImageBase64(base64Image string) (string, error) {

	log.Info().Str("component", "OCR_TESSERACT").Msg("Use tesseract with base 64")

	tmpFileName, err := createTempFileName("")
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

func (t TesseractEngine) tmpFileFromImageUrl(imgUrl string) (string, error) {

	log.Info().Str("component", "OCR_TESSERACT").Msg("Use tesseract with url")

	tmpFileName, err := createTempFileName("")
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

func (t TesseractEngine) processImageFile(inputFilename string, engineArgs TesseractEngineArgs) (OcrResult, error) {

	// if the input filename is /tmp/ocrimage, set the output file basename
	// to /tmp/ocrimage as well, which will produce /tmp/ocrimage.txt output
	tmpOutFileBaseName := inputFilename

	// possible file extensions
	fileExtensions := []string{"txt", "hocr", "json"}

	// build args array
	cflags := engineArgs.Export()
	cmdArgs := []string{inputFilename, tmpOutFileBaseName}
	cmdArgs = append(cmdArgs, cflags...)
	log.Info().Str("component", "OCR_TESSERACT").Interface("cmdArgs", cmdArgs)

	// exec tesseract
	cmd := exec.Command("tesseract", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Err(err).Str("component", "OCR_TESSERACT").Str("component", "OCR_TESSERACT").
			Msg(string(output))
		return OcrResult{Status: "error"}, err
	}

	outBytes, outFile, err := findAndReadOutfile(tmpOutFileBaseName, fileExtensions)

	// delete output file when we are done
	if engineArgs.saveFiles {
		defer os.Remove(outFile)
	}
	if err != nil {
		log.Error().Err(err).Str("component", "OCR_TESSERACT").
			Str("file_name", tmpOutFileBaseName).Msg("Error getting data from out file")
		return OcrResult{Status: "error"}, err
	}

	return OcrResult{
		Text:   string(outBytes),
		Status: "done",
	}, nil

}

func findOutfile(outfileBaseName string, fileExtensions []string) (string, error) {

	for _, fileExtension := range fileExtensions {

		outFile := fmt.Sprintf("%v.%v", outfileBaseName, fileExtension)
		log.Info().Str("component", "OCR_TESSERACT").Str("outFile", outFile).
			Msg("check if file exists")

		if _, err := os.Stat(outFile); err == nil {
			return outFile, nil
		}

	}

	return "", fmt.Errorf("Could not find outfile.  Basename: %v Extensions: %v", outfileBaseName, fileExtensions)

}

func findAndReadOutfile(outfileBaseName string, fileExtensions []string) (outBytes []byte, outfile string, err error) {

	outfile, err = findOutfile(outfileBaseName, fileExtensions)
	if err != nil {
		return nil, "", err
	}
	outBytes, err = ioutil.ReadFile(outfile)
	if err != nil {
		return nil, "", err
	}
	return outBytes, outfile, nil

}
