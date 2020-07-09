package utils

import (
	"fmt"
	"net/url"
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

// IsValidUrl checks if a string is a URL
func IsValidUrl(toTest string) bool {
	_, err := url.ParseRequestURI(toTest)
	if err != nil {
		return false
	}

	u, err := url.Parse(toTest)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	return true
}
