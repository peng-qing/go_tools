package encode_utils

import (
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

const (
	EncodingUTF8     = "UTF-8"
	EncodingUTF8BOM  = "UTF-8-BOM"
	EncodingGBK      = "GBK"
	EncodingGB18030  = "GB18030"
	EncodingHZGB2312 = "HZ-GB2312"
)

// NewEncoder 创建编码器
func NewEncoder(encodingStr string) *encoding.Encoder {
	switch encodingStr {
	case EncodingUTF8:
		return unicode.UTF8.NewEncoder()
	case EncodingUTF8BOM:
		return unicode.UTF8BOM.NewEncoder()
	case EncodingGBK:
		return simplifiedchinese.GBK.NewEncoder()
	case EncodingGB18030:
		return simplifiedchinese.GB18030.NewEncoder()
	case EncodingHZGB2312:
		return simplifiedchinese.HZGB2312.NewEncoder()
	default:
		return nil
	}
}

func NewDecoder(encodingStr string) *encoding.Decoder {
	switch encodingStr {
	case EncodingUTF8:
		return unicode.UTF8.NewDecoder()
	case EncodingUTF8BOM:
		return unicode.UTF8BOM.NewDecoder()
	case EncodingGBK:
		return simplifiedchinese.GBK.NewDecoder()
	case EncodingGB18030:
		return simplifiedchinese.GB18030.NewDecoder()
	case EncodingHZGB2312:
		return simplifiedchinese.HZGB2312.NewDecoder()
	default:
		return nil
	}
}
