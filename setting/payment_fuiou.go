// sudoapi: Fuiou payment.

package setting

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"

	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
)

var (
	FuiouPubKeyStr string
	FuiouPubKey    *rsa.PublicKey

	FuiouPriKeyStr string
	FuiouPriKey    *rsa.PrivateKey

	FuiouMerchant string
	FuiouUrl      string
	FuiouCallback string

	FuiouUnitPrice = decimal.NewFromFloat(7.3)
)

func FuiouEnabled() bool {
	return FuiouPubKey != nil && FuiouPriKey != nil &&
		FuiouUrl != "" && FuiouCallback != "" &&
		FuiouMerchant != ""
}

func SetFuiouPubKey(pubKeyStr string) error {
	FuiouPubKeyStr = pubKeyStr
	pubKeyBytes, err := base64.StdEncoding.DecodeString(pubKeyStr)
	if err != nil {
		return errors.Wrap(err, "decode pay pub key failed")
	}
	pubKey, err := x509.ParsePKIXPublicKey(pubKeyBytes)
	if err != nil {
		return errors.Wrap(err, "parse pay pub key failed")
	}
	FuiouPubKey = pubKey.(*rsa.PublicKey)
	return nil
}

func SetFuiouPriKey(priKeyStr string) error {
	FuiouPriKeyStr = priKeyStr
	privateKeyBytes, err := base64.StdEncoding.DecodeString(priKeyStr)
	if err != nil {
		return errors.Wrap(err, "decode pay pri key failed")
	}
	priKey, err := x509.ParsePKCS8PrivateKey(privateKeyBytes)
	if err != nil {
		return errors.Wrap(err, "parse pay pri key failed")
	}
	FuiouPriKey = priKey.(*rsa.PrivateKey)
	return nil
}
