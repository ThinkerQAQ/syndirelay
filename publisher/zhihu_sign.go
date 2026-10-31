package publisher

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
)

const (
	zhihuZSE93       = "101_3_3.0"
	zhihuZSEPrefix   = "2.0_"
	zhihuZSEAlphabet = "6fpLRqJO8M/c3jnYxFkUVC4ZIG12SiH=5v0mXDazWBTsuw7QetbKdoPyAl+hN9rgE"
	zhihuZSEKey16    = "059053f7d15e01d7"
	zhihuZSESeed     = byte(12)
)

var zhihuZSERoundKeys = [...]uint32{
	1170614578, 1024848638, 1413669199, 3951632832, 3528873006, 2921909214, 4151847688, 3997739139,
	1933479194, 3323781115, 3888513386, 460404854, 3747539722, 2403641034, 2615871395, 2119585428,
	2265697227, 2035090028, 2773447226, 4289380121, 4217216195, 2200601443, 3051914490, 1579901135,
	1321810770, 456816404, 2903323407, 4065664991, 330002838, 3506006750, 363569021, 2347096187,
}

var zhihuZSESBox = [...]byte{
	20, 223, 245, 7, 248, 2, 194, 209, 87, 6, 227, 253, 240, 128, 222, 91, 237, 9, 125, 157, 230, 93, 252, 205, 90, 79, 144, 199, 159, 197, 186, 167,
	39, 37, 156, 198, 38, 42, 43, 168, 217, 153, 15, 103, 80, 189, 71, 191, 97, 84, 247, 95, 36, 69, 14, 35, 12, 171, 28, 114, 178, 148, 86, 182,
	32, 83, 158, 109, 22, 255, 94, 238, 151, 85, 77, 124, 254, 18, 4, 26, 123, 176, 232, 193, 131, 172, 143, 142, 150, 30, 10, 146, 162, 62, 224, 218,
	196, 229, 1, 192, 213, 27, 110, 56, 231, 180, 138, 107, 242, 187, 54, 120, 19, 44, 117, 228, 215, 203, 53, 239, 251, 127, 81, 11, 133, 96, 204, 132,
	41, 115, 73, 55, 249, 147, 102, 48, 122, 145, 106, 118, 74, 190, 29, 16, 174, 5, 177, 129, 63, 113, 99, 31, 161, 76, 246, 34, 211, 13, 60, 68,
	207, 160, 65, 111, 82, 165, 67, 169, 225, 57, 112, 244, 155, 51, 236, 200, 233, 58, 61, 47, 100, 137, 185, 64, 17, 70, 234, 163, 219, 108, 170, 166,
	59, 149, 52, 105, 24, 212, 78, 173, 45, 0, 116, 226, 119, 136, 206, 135, 175, 195, 25, 92, 121, 208, 126, 139, 3, 75, 141, 21, 130, 98, 241, 40,
	154, 66, 184, 49, 181, 46, 243, 88, 101, 183, 8, 23, 72, 188, 104, 179, 210, 134, 250, 201, 164, 89, 216, 202, 220, 50, 221, 152, 140, 33, 235, 214,
}

func zhihuDC0FromSession(session Session) string {
	for _, cookie := range session.Cookies {
		if cookie.Name == "d_c0" && strings.TrimSpace(cookie.Value) != "" {
			return cookie.Value
		}
	}
	if value := zhihuCookieValueFromHeader(session.RequestCookieHeader, "d_c0"); value != "" {
		return value
	}
	for host, header := range session.RequestCookieHeaders {
		if requestHostAllowed(host, []string{"zhihu.com"}) {
			if value := zhihuCookieValueFromHeader(header, "d_c0"); value != "" {
				return value
			}
		}
	}
	return ""
}

func zhihuCookieValueFromHeader(header, name string) string {
	for _, pair := range strings.Split(header, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if ok && strings.TrimSpace(key) == name {
			return value
		}
	}
	return ""
}

func zhihuSignRequest(request *http.Request, dc0 string) error {
	if request == nil || request.URL == nil {
		return errors.New("invalid Zhihu request")
	}
	if !strings.EqualFold(request.URL.Hostname(), "www.zhihu.com") {
		return nil
	}
	if strings.TrimSpace(dc0) == "" {
		return errors.New("Zhihu d_c0 cookie is required for signed API requests")
	}

	digest := md5.Sum([]byte(zhihuZSE93 + "+" + request.URL.RequestURI() + "+" + dc0))
	request.Header.Set("x-zse-93", zhihuZSE93)
	request.Header.Set("x-zse-96", zhihuZSEPrefix+zhihuEncryptZSE(hex.EncodeToString(digest[:])))
	request.Header.Set("x-requested-with", "fetch")
	return nil
}

func zhihuEncryptZSE(md5Text string) string {
	plain := make([]byte, 0, 48)
	plain = append(plain, zhihuZSESeed, 0)
	plain = append(plain, md5Text...)
	padding := 16 - len(plain)%16
	for index := 0; index < padding; index++ {
		plain = append(plain, byte(padding))
	}

	first := make([]byte, 16)
	for index := range first {
		first[index] = plain[index] ^ zhihuZSEKey16[index] ^ 42
	}
	seed := zhihuZSEEncryptBlock(first)
	cipher := append([]byte{}, seed...)
	for offset := 16; offset < len(plain); offset += 16 {
		block := make([]byte, 16)
		for index := range block {
			block[index] = plain[offset+index] ^ seed[index]
		}
		seed = zhihuZSEEncryptBlock(block)
		cipher = append(cipher, seed...)
	}
	return zhihuZSEEncode(cipher)
}

func zhihuZSEEncryptBlock(input []byte) []byte {
	words := make([]uint32, 36)
	words[0] = zhihuZSEUint32(input, 0)
	words[1] = zhihuZSEUint32(input, 4)
	words[2] = zhihuZSEUint32(input, 8)
	words[3] = zhihuZSEUint32(input, 12)
	for index := 0; index < 32; index++ {
		words[index+4] = words[index] ^ zhihuZSETransform(words[index+1]^words[index+2]^words[index+3]^zhihuZSERoundKeys[index])
	}
	output := make([]byte, 16)
	zhihuZSEPutUint32(words[35], output, 0)
	zhihuZSEPutUint32(words[34], output, 4)
	zhihuZSEPutUint32(words[33], output, 8)
	zhihuZSEPutUint32(words[32], output, 12)
	return output
}

func zhihuZSETransform(value uint32) uint32 {
	bytes := []byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)}
	for index := range bytes {
		bytes[index] = zhihuZSESBox[bytes[index]]
	}
	result := zhihuZSEUint32(bytes, 0)
	return result ^ zhihuRotateLeft(result, 2) ^ zhihuRotateLeft(result, 10) ^ zhihuRotateLeft(result, 18) ^ zhihuRotateLeft(result, 24)
}

func zhihuZSEEncode(input []byte) string {
	data := append([]byte{}, input...)
	for len(data)%3 != 0 {
		data = append(data, 0)
	}
	result := strings.Builder{}
	result.Grow(len(data) / 3 * 4)
	position := len(data) - 1
	byteIndex := 0
	for position >= 0 {
		var value uint32
		for shift := 0; shift <= 16; shift += 8 {
			mask := byte((uint32(58) >> (8 * uint(byteIndex%4))) & 255)
			value |= uint32(data[position]^mask) << uint(shift)
			position--
			byteIndex++
		}
		for _, shift := range []uint{0, 6, 12, 18} {
			result.WriteByte(zhihuZSEAlphabet[(value>>shift)&63])
		}
	}
	return result.String()
}

func zhihuZSEUint32(input []byte, index int) uint32 {
	return uint32(input[index])<<24 | uint32(input[index+1])<<16 | uint32(input[index+2])<<8 | uint32(input[index+3])
}

func zhihuZSEPutUint32(value uint32, output []byte, index int) {
	output[index] = byte(value >> 24)
	output[index+1] = byte(value >> 16)
	output[index+2] = byte(value >> 8)
	output[index+3] = byte(value)
}

func zhihuRotateLeft(value uint32, shift uint) uint32 {
	return value<<shift | value>>(32-shift)
}
