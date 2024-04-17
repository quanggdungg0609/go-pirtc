package utils

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
)

func UploadImage(path string) error {
	if err := verifyPath(path); err != nil {
		return err
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	fileName := filepath.Base(path)
	requestBody := &bytes.Buffer{}
	writer := multipart.NewWriter(requestBody)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return err
	}
	return nil
}

func verifyPath(path string) error {
	isAbsolute := filepath.IsAbs(path)
	if !isAbsolute {
		return errors.New("IS NOT ABSOLUTE PATH")
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("FILE IS NOT EXIST")
		} else {
			return errors.New("FAILED TO VERIFY PATH")
		}
	}

	if !fileInfo.Mode().IsRegular() {
		return errors.New("PATH EXIST BUT IS NOT A FILE")
	}
	return nil
}
