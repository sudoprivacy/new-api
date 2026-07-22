// sudoapi: Fuiou payment.

package controller

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/pkg/errors"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
)

func RequestFuiouPay(c *gin.Context) {
	if !setting.FuiouEnabled() {
		c.JSON(200, gin.H{"message": "error", "data": "Fuiou payment not enabled"})
		return
	}

	var req struct {
		Amount        int64  `json:"amount"`
		PaymentMethod string `json:"payment_method"`
	}
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "Parameter error"})
		return
	}
	if req.Amount < getMinTopup() {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("The recharge quantity cannot be less than %d", getMinTopup())})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "Failed to obtain user groups"})
		return
	}

	payCents := fuiouPayMoneyCents(req.Amount, group)
	if payCents < 1 {
		c.JSON(200, gin.H{"message": "error", "data": "The recharge amount is too low"})
		return
	}

	var payType string
	switch req.PaymentMethod {
	case model.PaymentMethodFuiouAlipay:
		payType = "ALIPAY"
	case model.PaymentMethodFuiouWeChat:
		payType = "WECHAT"
	default:
		c.JSON(200, gin.H{"message": "error", "data": "Payment method does not exist"})
		return
	}

	fuiouUrl, err := url.Parse(setting.FuiouUrl + "/aggpos/order.fuiou")
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "payment failed"})
		return
	}
	notifyUrl, err := url.Parse(setting.FuiouCallback + "/api/fuiou/callback")
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "payment failed"})
		return
	}

	now := time.Now()
	// 数字和英文组合 30位
	tradeNo := fmt.Sprintf("U%dT%d%s", 4635, now.Unix(), common.GetRandomString(6))
	name := fmt.Sprintf("NAPI %d", req.Amount)

	message, err := purchaseFuiou(RequestMessage{
		url:           fuiouUrl.String(),
		pubKey:        setting.FuiouPubKey,
		priKey:        setting.FuiouPriKey,
		MchntCD:       setting.FuiouMerchant,
		OrderDate:     now.Format("20060102"),
		OrderID:       tradeNo,
		OrderAmt:      strconv.FormatInt(payCents, 10),
		OrderPayType:  payType,
		BackNotifyUrl: notifyUrl.String(),
		GoodsName:     name,
		GoodsDetail:   name,
		Ver:           "1.0.0",
	})
	if err != nil {
		logger.LogError(c, fmt.Sprintf("fuiou topup failed, err: %+v", err))
		c.JSON(200, gin.H{"message": "error", "data": "Pull up payment failed"})
		return
	}

	topUp := &model.TopUp{
		UserId:          id,
		Amount:          req.Amount,
		Money:           float64(req.Amount),
		TradeNo:         tradeNo,
		PaymentMethod:   req.PaymentMethod,
		PaymentProvider: model.PaymentProviderFuiou,
		CreateTime:      now.Unix(),
		Status:          common.TopUpStatusPending,
	}
	err = topUp.Insert()
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "Order creation failed"})
		return
	}
	c.JSON(200, gin.H{"message": "success", "data": gin.H{
		"order_date": message.OrderDate,
		"order_amt":  message.OrderAmt,
		"order_id":   message.OrderID,
		"order_info": message.OrderInfo,
	}})
}

func RequestFuiouAmount(c *gin.Context) {
	if !setting.FuiouEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Fuiou payment not enabled"})
		return
	}

	var req AmountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Parameter error"})
		return
	}

	if minTopup := getMinTopup(); req.Amount < minTopup {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("The recharge quantity cannot be less than %d", minTopup)})
		return
	}

	group, err := model.GetUserGroup(c.GetInt("id"), true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Failed to obtain user groups"})
		return
	}
	payCents := fuiouPayMoneyCents(req.Amount, group)
	if payCents < 1 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "The recharge amount is too low"})
		return
	}

	payMoney := decimal.NewFromInt(payCents).Div(hundred)
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": payMoney.StringFixed(2)})
}

func GetTopUp(c *gin.Context) {
	topUp, err := model.GetTopUp(c.Param("orderID"), c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"order_id":     topUp.TradeNo,
		"order_status": topUp.Status,
	})
}

func FuiouCallback(c *gin.Context) {
	payload, err := io.ReadAll(c.Request.Body)
	log.Printf("fuiou callback message: %s", string(payload))
	if err != nil {
		log.Printf("parse Fuiou callback data failed: %v", err)
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	var resp Response
	if err = common.Unmarshal(payload, &resp); err != nil {
		log.Printf("unmarshal Fuiou callback data failed: %v", err)
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	if resp.RespCode != "0000" {
		log.Printf("invalid Fuiou callback data, code: %s, desc: %s", resp.RespCode, resp.RespDesc)
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	respMsgBytes, err := decrypt(setting.FuiouPriKey, resp.Message)
	if err != nil {
		log.Printf("decrypt Fuiou callback data failed: %v", err)
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	var respMsg CallbackMessage
	if err = common.Unmarshal(respMsgBytes, &respMsg); err != nil {
		log.Printf("unmarshal Fuiou callback message failed: %v", err)
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	switch respMsg.OrderSt {
	case "1":
		err = model.RechargeCommon(respMsg.OrderID, string(respMsgBytes))
		if err != nil {
			log.Printf("recharge failed: %v", err)
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
	case "2":
		err = model.ExpireTopUp(respMsg.OrderID)
		if err != nil {
			log.Printf("expire top-up failed: %v", err)
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
	}
	c.Status(http.StatusOK)
}

var hundred = decimal.NewFromInt(100)

func fuiouPayMoneyCents(amountInt64 int64, group string) int64 {
	ratio := common.GetTopupGroupRatio(group)
	if ratio == 0 {
		ratio = 1
	}

	return decimal.NewFromInt(amountInt64).
		Mul(setting.FuiouUnitPrice).      // to rmb
		Mul(hundred).                     // to cents
		Mul(decimal.NewFromFloat(ratio)). // with ratio
		IntPart()
}

type RequestMessage struct {
	url    string
	pubKey *rsa.PublicKey
	priKey *rsa.PrivateKey

	MchntCD       string `json:"mchnt_cd"`
	OrderDate     string `json:"order_date"`
	OrderID       string `json:"order_id"`
	OrderAmt      string `json:"order_amt"`
	OrderPayType  string `json:"order_pay_type"`
	BackNotifyUrl string `json:"back_notify_url"`
	GoodsName     string `json:"goods_name"`
	GoodsDetail   string `json:"goods_detail"`
	Ver           string `json:"ver"`
}

type Response struct {
	MchntCD  string `json:"mchnt_cd"`
	Message  []byte `json:"message"`
	RespCode string `json:"resp_code"`
	RespDesc string `json:"resp_desc"`
}

type ResponseMessage struct {
	OrderDate    string `json:"order_date"`
	OrderPayType string `json:"order_pay_type"`
	OrderAmt     string `json:"order_amt"`
	MchntCd      string `json:"mchnt_cd"`
	OrderID      string `json:"order_id"`
	OrderInfo    string `json:"order_info"`
}

func purchaseFuiou(message RequestMessage) (*ResponseMessage, error) {
	type Request struct {
		MchntCD string `json:"mchnt_cd"`
		Message []byte `json:"message"`
	}

	reqMsg, err := common.Marshal(&message)
	if err != nil {
		return nil, errors.Wrap(err, "marshal order message failed")
	}
	log.Printf("fuiou request message: %s", string(reqMsg))
	reqMsgBytes, err := encrypt(message.pubKey, reqMsg)
	if err != nil {
		return nil, errors.Wrap(err, "encrypt order message failed")
	}

	body, err := common.Marshal(&Request{MchntCD: message.MchntCD, Message: reqMsgBytes})
	if err != nil {
		return nil, errors.Wrap(err, "marshal order request failed")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Post(message.url, "application/json;charset=UTF-8", bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "post order request failed")
	}
	defer func() { _ = response.Body.Close() }()

	var resp Response
	if err = common.DecodeJson(response.Body, &resp); err != nil {
		return nil, errors.Wrap(err, "unmarshal order response failed")
	}
	if resp.RespCode != "0000" {
		return nil, errors.Errorf("order failed, code: %s, desc: %s", resp.RespCode, resp.RespDesc)
	}

	respMsgBytes, err := decrypt(message.priKey, resp.Message)
	if err != nil {
		return nil, errors.Wrap(err, "decrypt order response failed")
	}
	var respMsg ResponseMessage
	if err = common.Unmarshal(respMsgBytes, &respMsg); err != nil {
		return nil, errors.Wrap(err, "unmarshal order message failed")
	}

	return &respMsg, nil
}

func encrypt(pubKey *rsa.PublicKey, data []byte) ([]byte, error) {
	chunkSize := pubKey.Size() - 11
	buf := new(bytes.Buffer)
	for i := 0; i < len(data); i += chunkSize {
		msg, err := rsa.EncryptPKCS1v15(rand.Reader, pubKey, data[i:min(i+chunkSize, len(data))])
		if err != nil {
			return nil, err
		}
		buf.Write(msg)
	}
	return buf.Bytes(), nil
}

func decrypt(priKey *rsa.PrivateKey, data []byte) ([]byte, error) {
	chunkSize := priKey.Size()
	buf := new(bytes.Buffer)
	for i := 0; i < len(data); i += chunkSize {
		msg, err := rsa.DecryptPKCS1v15(rand.Reader, priKey, data[i:min(i+chunkSize, len(data))])
		if err != nil {
			return nil, err
		}
		buf.Write(msg)
	}
	return buf.Bytes(), nil
}

type CallbackMessage struct {
	OrderID string `json:"order_id"`
	OrderSt string `json:"order_st"`
}
