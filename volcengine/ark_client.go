// sudoapi: Volcengine ark asset.

package volcengine

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tidwall/gjson"

	"github.com/QuantumNous/new-api/service"
)

const (
	region      = "cn-beijing"
	serviceName = "ark"
	version     = "2024-01-01"
)

func callArk[Result any](ctx context.Context, action string, req any) (*Result, error) {
	var reqBody io.Reader
	if reader, ok := req.(io.Reader); ok {
		reqBody = reader
	} else {
		body, err := json.Marshal(req)
		if err != nil {
			return nil, errors.Wrap(err, "marshal ark request failed")
		}
		reqBody = bytes.NewReader(body)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, arkSetting.BaseURL, reqBody)
	if err != nil {
		return nil, errors.Wrap(err, "create ark request failed")
	}
	query := request.URL.Query()
	query.Set("Action", action)
	query.Set("Version", version)
	request.URL.RawQuery = query.Encode()

	if err = signRequest(request, arkSetting.AK, arkSetting.SK); err != nil {
		return nil, errors.Wrap(err, "sign ark request failed")
	}

	httpClient, err := service.GetHttpClientWithProxy(arkSetting.ProxyURL)
	if err != nil {
		return nil, errors.Wrap(err, "create ark http client failed")
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, errors.Wrap(err, "send ark request failed")
	}

	defer func() { _ = response.Body.Close() }()
	respBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read ark response failed")
	}
	if response.StatusCode != http.StatusOK {
		return nil, errors.Errorf("ark returned http %d: %s", response.StatusCode, string(respBody))
	}

	resp := gjson.ParseBytes(respBody)
	if resp.Get("success").Type == gjson.False {
		return nil, newError(resp.Get("code").String(), resp.Get("message").String())
	}
	if errorPart := resp.Get("ResponseMetadata.Error"); errorPart.IsObject() {
		return nil, newError(errorPart.Get("Code").String(), errorPart.Get("Message").String())
	}
	rstPart := resp.Get("Result")
	if !rstPart.IsObject() {
		return nil, errors.Errorf("unknown ark result: %s", string(respBody))
	}
	var rst Result
	if err = json.Unmarshal([]byte(rstPart.Raw), &rst); err != nil {
		return nil, errors.Wrap(err, "unmarshal ark result failed")
	}
	return &rst, nil
}

func signRequest(req *http.Request, accessKey, secretKey string) error {
	var bodyBytes []byte
	var err error

	if req.Body != nil {
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return errors.Wrap(err, "read request body failed")
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	payloadHash := sha256.Sum256(bodyBytes)
	hexPayloadHash := hex.EncodeToString(payloadHash[:])

	t := time.Now().UTC()
	xDate := t.Format("20060102T150405Z")
	shortDate := t.Format("20060102")

	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Date", xDate)
	req.Header.Set("X-Content-Sha256", hexPayloadHash)
	req.Header.Set("Content-Type", "application/json")

	queryParams := req.URL.Query()
	sortedKeys := make([]string, 0, len(queryParams))
	for key := range queryParams {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)
	var queryParts []string
	for _, key := range sortedKeys {
		values := queryParams[key]
		sort.Strings(values)
		for _, value := range values {
			queryParts = append(queryParts, fmt.Sprintf("%s=%s", url.QueryEscape(key), url.QueryEscape(value)))
		}
	}
	canonicalQueryString := strings.Join(queryParts, "&")

	headersToSign := map[string]string{
		"host":             req.URL.Host,
		"x-date":           xDate,
		"x-content-sha256": hexPayloadHash,
		"content-type":     "application/json",
	}

	signedHeaderKeys := make([]string, 0, len(headersToSign))
	for key := range headersToSign {
		signedHeaderKeys = append(signedHeaderKeys, key)
	}
	sort.Strings(signedHeaderKeys)

	var canonicalHeaders strings.Builder
	for _, key := range signedHeaderKeys {
		canonicalHeaders.WriteString(key)
		canonicalHeaders.WriteString(":")
		canonicalHeaders.WriteString(strings.TrimSpace(headersToSign[key]))
		canonicalHeaders.WriteString("\n")
	}
	signedHeaders := strings.Join(signedHeaderKeys, ";")

	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		req.Method,
		req.URL.Path,
		canonicalQueryString,
		canonicalHeaders.String(),
		signedHeaders,
		hexPayloadHash,
	)

	hashedCanonicalRequest := sha256.Sum256([]byte(canonicalRequest))
	hexHashedCanonicalRequest := hex.EncodeToString(hashedCanonicalRequest[:])

	credentialScope := fmt.Sprintf("%s/%s/%s/request", shortDate, region, serviceName)
	stringToSign := fmt.Sprintf("HMAC-SHA256\n%s\n%s\n%s", xDate, credentialScope, hexHashedCanonicalRequest)

	kDate := hmacSHA256([]byte(secretKey), []byte(shortDate))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(serviceName))
	kSigning := hmacSHA256(kService, []byte("request"))
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	authorization := fmt.Sprintf("HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey,
		credentialScope,
		signedHeaders,
		signature,
	)
	req.Header.Set("Authorization", authorization)
	return nil
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(data)
	return h.Sum(nil)
}
