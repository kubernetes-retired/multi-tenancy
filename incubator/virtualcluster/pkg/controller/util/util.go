package util

import apierrors "k8s.io/apimachinery/pkg/api/errors"

func IgnoreNotFound(err error) error {
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func ContainString(sli []string, s string) bool {
	for _, str := range sli {
		if str == s {
			return true
		}
	}
	return false
}

func RemoveString(sli []string, s string) (newSli []string) {
	for _, str := range sli {
		if str == s {
			continue
		}
		newSli = append(newSli, str)
	}
	return
}
