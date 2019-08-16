package ocrworker

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/rs/zerolog/log"
)

type StrokeWidthTransformer struct {
}

func (s StrokeWidthTransformer) preprocess(ocrRequest *OcrRequest) error {

	// write bytes to a temp file

	tmpFileNameInput, err := createTempFileName()
	tmpFileNameInput = fmt.Sprintf("%s.png", tmpFileNameInput)
	if err != nil {
		return err
	}
	defer os.Remove(tmpFileNameInput)

	tmpFileNameOutput, err := createTempFileName()
	tmpFileNameOutput = fmt.Sprintf("%s.png", tmpFileNameOutput)
	if err != nil {
		return err
	}
	defer os.Remove(tmpFileNameOutput)

	err = saveBytesToFileName(ocrRequest.ImgBytes, tmpFileNameInput)
	if err != nil {
		return err
	}

	// run DecodeText binary on it (if not in path, print warning and do nothing)
	darkOnLightSetting := s.extractDarkOnLightParam(*ocrRequest)
	log.Info().Str("component", "PREPROCESSOR_WORKER").
		Str("tmpFileNameInput", tmpFileNameInput).Str("tmpFileNameOutput", tmpFileNameOutput).
		Str("darkOnLightSetting", darkOnLightSetting).Msg("DetectText")

	out, err := exec.Command(
		"DetectText",
		tmpFileNameInput,
		tmpFileNameOutput,
		darkOnLightSetting,
	).CombinedOutput()
	if err != nil {
		log.Error().Err(err).Msg(string(out))
	}

	// read bytes from output file into ocrRequest.ImgBytes
	resultBytes, err := ioutil.ReadFile(tmpFileNameOutput)
	if err != nil {
		return err
	}

	ocrRequest.ImgBytes = resultBytes

	return nil

}

func (s StrokeWidthTransformer) extractDarkOnLightParam(ocrRequest OcrRequest) string {

	log.Info().Str("component", "PREPROCESSOR_WORKER").
		Msg("extract dark on light param")

	val := "1"

	preprocessorArgs := ocrRequest.PreprocessorArgs
	swtArgs := preprocessorArgs[PreprocessorStrokeWidthTransform]
	if swtArgs != nil {
		swtArg, ok := swtArgs.(string)
		if ok && (swtArg == "0" || swtArg == "1") {
			val = swtArg
		}
	}

	log.Info().Str("component", "PREPROCESSOR_WORKER").Str("val", val).Msg("return value")

	return val
}
