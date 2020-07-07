package utils

import (
	"fmt"
	"os"
	"strings"
)

func Exists(path string) (bool, error) {

	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}

	return false, err
}

func GetDirectory(path string, delimiter string) string {

	dir := strings.Split(path, delimiter)
	dir = dir[0 : len(dir)-1]
	dirPath := strings.Join(dir[:], "/")

	return dirPath
}

func CheckError(err error) {
	if err != nil {
		fmt.Println("Fatal error ", err.Error())
		os.Exit(1)
	}
}
