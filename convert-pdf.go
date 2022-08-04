package ocrworker

/*	Use this module if you want to call tesseract over
	pdfsandwich with an image as input file.
	Useful with big documents.

	Use cases:
	engine: tesseract with file_type: pdf and preprocessor: convert-pdf
	engine: sandwich  with file_type: [tif, png, jpg] and preprocessor: convert-pdf
*/

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/rs/zerolog/log"
)

type ConvertPdf struct{}

func (ConvertPdf) preprocess(ocrRequest *OcrRequest) error {
	tmpFileNameInput, err := createTempFileName("")
	tmpFileNameInput = fmt.Sprintf("%s.pdf", tmpFileNameInput)
	if err != nil {
		return err
	}
	defer func(name string) {
		err := os.Remove(name)
		if err != nil {
			log.Warn().Err(err).Str("component", "PREPROCESSOR_WORKER").Msg(name + " could not be removed")
		}
	}(tmpFileNameInput)

	tmpFileNameOutput, err := createTempFileName("")
	tmpFileNameOutput = fmt.Sprintf("%s.tif", tmpFileNameOutput)
	if err != nil {
		return err
	}
	defer func(name string) {
		err := os.Remove(name)
		if err != nil {
			log.Warn().Err(err).Str("component", "PREPROCESSOR_WORKER").Msg(name + " could not be removed")
		}
	}(tmpFileNameOutput)

	err = saveBytesToFileName(ocrRequest.ImgBytes, tmpFileNameInput)
	if err != nil {
		return err
	}

	log.Info().Str("component", "PREPROCESSOR_WORKER").Str("tmpFileNameInput", tmpFileNameInput).
		Str("tmpFileNameOutput", tmpFileNameOutput).Msg("Convert PDF")

	var gsArgs []string
	gsArgs = append(gsArgs,
		"-dQUIET",
		"-dNOPAUSE",
		"-dBATCH",
		"-sOutputFile="+tmpFileNameOutput,
		"-sDEVICE=tiffg4",
		tmpFileNameInput,
	)
	log.Info().Str("component", "PREPROCESSOR_WORKER").Interface("gsArgs", gsArgs)

	out, err := exec.Command("gs", gsArgs...).CombinedOutput()
	if err != nil {
		log.Error().Err(err).Str("component", "PREPROCESSOR_CONVERTPDF").Msg(string(out))
	}

	// read bytes from output file
	resultBytes, err := os.ReadFile(tmpFileNameOutput)
	if err != nil {
		return err
	}
	ocrRequest.ImgBytes = resultBytes

	return nil
}
