/*
Copyright 2021 The KodeRover Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/koderover/zadig/v2/pkg/tool/log"

	"github.com/koderover/zadig/v2/pkg/config"
)

func GenerateTmpFile() (string, error) {
	var tmpFile *os.File

	tmpFile, err := os.CreateTemp("", "")
	if err != nil {
		return "", err
	}

	_ = tmpFile.Close()

	return tmpFile.Name(), nil
}

func CreateFileInCurrentDir(filename string) (string, error) {
	//dir, err := os.Getwd()
	//if err != nil {
	//	return "", err
	//}

	if _, err := os.Stat(config.VMTaskLogPath()); os.IsNotExist(err) {
		os.MkdirAll("config.VMTaskLogPath()", os.ModePerm)
	}

	filePath := filepath.Join(config.VMTaskLogPath(), filename)
	log.Infof("----------- staring create log file: %s", filePath)
	f, err := os.Create(filePath)
	if err != nil {
		log.Errorf("-- failed to create log file: %s", err)
		return "", err
	}
	defer f.Close()

	return filePath, nil
}

func WriteFile(filename string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR|os.O_APPEND, perm)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

func ReadFile(filename string) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	contentByte, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return contentByte, nil
}

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
