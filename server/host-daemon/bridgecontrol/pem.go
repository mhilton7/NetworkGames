package bridgecontrol

import "encoding/pem"

func decodeCertificate(value []byte) ([]byte, []byte) {
	block, rest := pem.Decode(value)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, rest
	}
	return block.Bytes, rest
}
