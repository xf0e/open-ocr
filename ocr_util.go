package ocrworker

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/segmentio/ksuid"
)

func saveUrlContentToFileName(uri, tmpFileName string) error {

	outFile, err := os.Create(tmpFileName)
	if err != nil {
		return err
	}
	defer func(outFile *os.File) {
		err := outFile.Close()
		if err != nil {
			log.Warn().Err(err).Caller().Str("component", "OCR_UTIL").Msg(outFile.Name() + " could not be closed")
		}
	}(outFile)

	resp, err := http.Get(uri)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Warn().Err(err).Caller().Str("component", "OCR_UTIL").Msg(resp.Request.RequestURI + " request Body could not be closed")
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return err
	}

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		log.Warn().Err(err).Caller().Str("component", "OCR_UTIL").Msg(outFile.Name() + " could not be written")
		return err
	}
	return err
}

func saveBytesToFileName(bytes []byte, tmpFileName string) error {
	return os.WriteFile(tmpFileName, bytes, 0600)
}

func url2bytes(uri string) ([]byte, error) {

	var client = &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(uri)
	if err != nil {
		return nil, err
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Warn().Err(err).Caller().Str("component", "OCR_UTIL").Msg(resp.Request.RequestURI + " could not be closed")
		}
	}(resp.Body)

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Warn().Err(err).Caller().Str("component", "OCR_UTIL").Msg("request from " + resp.Request.RequestURI + " could not be read")
		return nil, err
	}

	return bodyBytes, nil

}

// createTempFileName generating a file name within of a temp directory. If function argument ist empty string
// file name will be generated in ksuid format.
func createTempFileName(fileName string) (string, error) {
	tempDir := os.TempDir()

	if fileName == "" {
		ksuidRaw := ksuid.New()
		fileName = ksuidRaw.String()
	}

	return filepath.Join(tempDir, fileName), nil
}

func readFirstBytes(filePath string, nBytesToRead uint) ([]byte, error) {

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Warn().Err(err).Caller().Str("component", "OCR_UTIL").Msg(file.Name() + " could not be closed")
		}
	}(file)

	buffer := make([]byte, nBytesToRead)
	_, err = file.Read(buffer)
	if err != nil {
		return nil, err
	}
	return buffer, nil
}

// detect uploaded file type
func detectFileType(buffer []byte) string {
	log.Info().Str("component", "OCR_DETECTFILETYPE").
		Interface("buffer", buffer).
		Msg("check file type; see buffer")
	fileType := ""
	switch {
	case len(buffer) > 3 &&
		buffer[0] == 0x25 && buffer[1] == 0x50 &&
		buffer[2] == 0x44 && buffer[3] == 0x46:
		fileType = strings.ToUpper("PDF")
	case len(buffer) > 3 &&
		((buffer[0] == 0x49 && buffer[1] == 0x49 && buffer[2] == 0x2A && buffer[3] == 0x0) ||
			(buffer[0] == 0x4D && buffer[1] == 0x4D && buffer[2] == 0x0 && buffer[3] == 0x2A)):
		fileType = strings.ToUpper("TIFF")
	default:
		fileType = strings.ToUpper("UNKNOWN")
	}
	return fileType
}

// if sandwich engine gets a TIFF image instead of PDF file
// we need to convert the input file to pdf first since pdfsandwich can't handle images
func convertImageToPdf(inputFilename string) string {
	log.Info().Str("component", "OCR_IMAGECONVERT").Msg("got image file instead of pdf, trying to convert it...")

	tmpFileImgToPdf := fmt.Sprintf("%s%s", inputFilename, ".pdf")
	cmd := exec.Command("convert", inputFilename, tmpFileImgToPdf)
	_, err := cmd.CombinedOutput()
	if err != nil {
		log.Debug().Err(err).Str("component", "OCR_IMAGECONVERT").Interface("tiff2pdf_args", cmd.Args)
		log.Warn().Err(err).Str("component", "OCR_IMAGECONVERT").Err(err).
			Msg("error exec convert for transforming TIFF to PDF")
		return ""
	}

	return tmpFileImgToPdf

}

// if sandwich engine gets a TIFF image instead of PDF file
// we need to convert the input file to pdf first since pdfsandwich can't handle images
// in this case tiff2pdf will be used; seems to be more reliable
func tiff2Pdf(inputFilename string) string {
	log.Info().Str("component", "OCR_IMAGECONVERT").Msg("got image file instead of pdf, trying to tiff2pdf it...")

	tmpFileImgToPdf := fmt.Sprintf("%s%s", inputFilename, ".pdf")
	cmd := exec.Command("tiff2pdf", inputFilename, "-o", tmpFileImgToPdf)
	_, err := cmd.CombinedOutput()
	if err != nil {
		log.Debug().Err(err).Str("component", "OCR_IMAGECONVERT").Interface("tiff2pdf_args", cmd.Args)
		log.Warn().Err(err).Str("component", "OCR_IMAGECONVERT").Err(err).
			Msg("error exec tiff2pdf for transforming TIFF to PDF")
		return ""
	}

	return tmpFileImgToPdf
}

// checkURLForReplyTo Checks if provided string is a valid URL
func checkURLForReplyTo(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		errorText := "provided " + u.String() + " URI must be an absolute URL"
		err = fmt.Errorf(errorText)
	}
	return u.String(), err
}

// timeTrack used to measure time of selected operations
func timeTrack(start time.Time, operation, message, requestID string) {
	elapsed := time.Since(start)
	if requestID == "" {
		log.Info().Str("component", "ocr_worker").Dur(operation, elapsed).
			Timestamp().Msg(message)
	}
	log.Info().Str("component", "ocr_worker").Dur(operation, elapsed).
		Str("RequestID", requestID).Timestamp().Msg(message)
}

// StripPasswordFromUrl strips passwords from URL
func StripPasswordFromUrl(urlToLog *url.URL) string {

	pass, passSet := urlToLog.User.Password()

	if passSet {
		return strings.Replace(urlToLog.String(), pass+"@", "***@", 1)
	}
	return urlToLog.String()
}
