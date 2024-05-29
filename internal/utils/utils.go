package utils

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func escapeQuotes(s string) string {
	return quoteEscaper.Replace(s)
}

func GetCurrentTimeStr() string {
	currentTime := time.Now()

	hour := currentTime.Hour()
	minute := currentTime.Minute()
	second := currentTime.Second()
	day := currentTime.Day()
	month := currentTime.Month()
	year := currentTime.Year()

	timeString := fmt.Sprintf("%02d%02d%02d_%02d%02d%d", hour, minute, second, day, month, year)

	return timeString
}

func UploadImage(uri string, path string, apiKey string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	requestBody := &bytes.Buffer{}
	writer := multipart.NewWriter(requestBody)

	part, err := createFormFileImage(writer, "file", filepath.Base(path))
	if err != nil {
		return err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return err
	}
	writer.Close()

	req, err := http.NewRequest("POST", uri, requestBody)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-API-KEY", apiKey)
	
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response: %s", resp.Status)
	}
	return nil
}

func UploadVideo(uri string, path string, camUuid string, apiKey string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	requestBody := &bytes.Buffer{}
	writer := multipart.NewWriter(requestBody)

	err = writer.WriteField("camera-uuid", camUuid)
	if err != nil {
		return err
	}

	part, err := createFormFileVideo(writer, "file", filepath.Base(path))
	if err != nil {
		return err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return err
	}
	writer.Close()

	req, err := http.NewRequest("POST", uri, requestBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-API-KEY", apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response: %s", resp.Status)
	}
	return nil
}

func createFormFileImage(w *multipart.Writer, fieldname string, filename string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s";  filename="%s"`, escapeQuotes(fieldname), escapeQuotes(filename)))
	h.Set("Content-Type", "image/jpeg")
	return w.CreatePart(h)
}

func createFormFileVideo(w *multipart.Writer, fieldname string, filename string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s";  filename="%s"`, escapeQuotes(fieldname), escapeQuotes(filename)))
	h.Set("Content-Type", "video/webm")
	return w.CreatePart(h)
}
