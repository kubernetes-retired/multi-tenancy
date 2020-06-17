package util

import (
	"fmt"
	"os"
	"strings"
)

func Exists(path string) (bool, error) {

	_, err := os.Stat(path)
	fmt.Println(err)
	if err == nil {
		return true, nil
	}

	return false, err

}

func GetDirectory(path string, delimiter string) string {

	dir := strings.Split(path, delimiter)
	testDir := dir[len(dir)-1]

	return testDir
}
