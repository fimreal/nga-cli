package client

import (
	"testing"
)

func TestGBKEncodeDecode(t *testing.T) {
	original := "这是一段测试 NGA 中文与特殊符号：·¥#！"
	gbkBytes, err := EncodeGBK(original)
	if err != nil {
		t.Fatalf("failed to encode GBK: %v", err)
	}

	decodedBytes, err := DecodeGBK(gbkBytes)
	if err != nil {
		t.Fatalf("failed to decode GBK: %v", err)
	}

	if string(decodedBytes) != original {
		t.Errorf("expected %q, got %q", original, string(decodedBytes))
	}
}
