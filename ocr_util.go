package ocrworker

import (
	"fmt"
	"github.com/couchbaselabs/logg"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nu7hatch/gouuid"
)

func saveUrlContentToFileName(url, tmpFileName string) error {

	// TODO: current impl uses more memory than it needs to

	resp, err := http.Get(url)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(tmpFileName, bodyBytes, 0600)

}

func saveBytesToFileName(bytes []byte, tmpFileName string) error {
	return ioutil.WriteFile(tmpFileName, bytes, 0600)
}

func url2bytes(url string) ([]byte, error) {

	var client = &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return bodyBytes, nil

}

func createTempFileName() (string, error) {
	tempDir := os.TempDir()
	uuidRaw, err := uuid.NewV4()
	if err != nil {
		return "", err
	}
	uuidStr := uuidRaw.String()
	return filepath.Join(tempDir, uuidStr), nil
}

func readFirstBytes(filePath string, nBytesToRead uint) ([]byte, error) {

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	buffer := make([]byte, nBytesToRead)
	_, err = file.Read(buffer)
	if err != nil {
		return nil, err
	}
	return buffer, nil
}

// detect uploaded file type
func detectFileType(buffer []byte) string {
	logg.LogTo("OCR_SANDWICH", "OK, this is buffer: %v", buffer)
	fileType := ""
	if len(buffer) > 3 &&
		buffer[0] == 0x25 && buffer[1] == 0x50 &&
		buffer[2] == 0x44 && buffer[3] == 0x46 {
		fileType = strings.ToUpper("PDF")
	} else if len(buffer) > 3 &&
		((buffer[0] == 0x49 && buffer[1] == 0x49 && buffer[2] == 0x2A && buffer[3] == 0x0) ||
			(buffer[0] == 0x4D && buffer[1] == 0x4D && buffer[2] == 0x0 && buffer[3] == 0x2A)) {
		fileType = strings.ToUpper("TIFF")
	} else {
		fileType = strings.ToUpper("UNKNOWN")
	}
	return fileType
}

// if sandwich engine gets a TIFF image instead of PDF file
// we need to convert the input file to pdf first since pdfsandwich can't handle images
func convertImageToPdf(inputFilename string) string {
	logg.LogTo("OCR_SANDWICH", "got image file instead of pdf, trying to convert it...")

	tmpFileImgToPdf := fmt.Sprintf("%s%s", inputFilename, ".pdf")
	cmd := exec.Command("convert", inputFilename, tmpFileImgToPdf)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logg.LogWarn("OCR_SANDWICH", "error exec convert for transforming TIFF to PDF: %v %v", err, string(output))
		return ""
	}

	return tmpFileImgToPdf

}

// checkURLForReplyTo Checks if provided string is a valid URL
func checkURLForReplyTo(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		logg.LogWarn("OCR_CLIENT", "provided("+u.String()+") URI address is not valid")
		return "", err
	} else if u.Scheme == "" || u.Host == "" {
		errorText := "provided(" + u.String() + ") URI must be an absolute URL"
		err = fmt.Errorf(errorText)
	} else if u.Scheme != "http" {
		errorText := "provided(" + u.String() + ") URI must begin with http"
		err = fmt.Errorf(errorText)
	}
	return u.String(), err
}
