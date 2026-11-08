package client

import (
	"bytes"
	"io"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// DecodeGBK decodes GBK/GB18030 encoded bytes to UTF-8.
func DecodeGBK(src []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(src), simplifiedchinese.GB18030.NewDecoder())
	return io.ReadAll(reader)
}

// EncodeGBK encodes UTF-8 string to GBK/GB18030 encoded bytes.
func EncodeGBK(src string) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader([]byte(src)), simplifiedchinese.GB18030.NewEncoder())
	return io.ReadAll(reader)
}
