// sudoapi: Volcengine ark asset.

package volcengine

import (
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
)

type Error struct {
	code string
	msg  string
}

func (e Error) Error() string {
	return fmt.Sprintf("code: %s, msg: %s", e.code, e.msg)
}

func newError(code string, msg string) Error {
	return Error{code: code, msg: msg}
}

type (
	CreateAssetGroupRequest struct {
		Name        string    `json:"Name"`
		Description string    `json:"Description,omitzero"`
		GroupType   GroupType `json:"GroupType,omitzero"`
		ProjectName string    `json:"ProjectName,omitzero"`
	}
	CreateAssetGroupResponse struct {
		ID string `json:"Id"`
	}

	CreateAssetRequest struct {
		GroupID     string `json:"GroupId,omitzero"`
		URL         string `json:"URL"`
		Name        string `json:"Name,omitzero"`
		AssetType   string `json:"AssetType,omitzero"`
		ProjectName string `json:"ProjectName,omitzero"`
	}
	CreateAssetResponse struct {
		ID      string `json:"Id"`
		AssetID string `json:"AssetId,omitzero"`
	}
)

type (
	ListResponse[Item any] struct {
		TotalCount int64  `json:"TotalCount"`
		PageSize   int    `json:"PageSize"`
		PageNumber int    `json:"PageNumber"`
		Items      []Item `json:"Items,omitzero"`
	}
	Filter struct {
		GroupIDs  []string  `json:"GroupIds,omitzero"`
		GroupType GroupType `json:"GroupType,omitzero"`
		Name      string    `json:"Name,omitzero"`
		Statuses  []string  `json:"Statuses,omitzero"`
	}
	ListRequest struct {
		Filter      Filter `json:"Filter,omitzero"`
		PageNumber  int    `json:"PageNumber,omitzero"`
		PageSize    int    `json:"PageSize,omitzero"`
		SortBy      string `json:"SortBy,omitzero"`
		SortOrder   string `json:"SortOrder,omitzero"`
		ProjectName string `json:"ProjectName,omitzero"`
	}

	AssetGroup struct {
		ID          string    `json:"Id"`
		Name        string    `json:"Name"`
		Description string    `json:"Description"`
		GroupType   GroupType `json:"GroupType"`
		ProjectName string    `json:"ProjectName"`
		CreateTime  time.Time `json:"CreateTime"`
		UpdateTime  time.Time `json:"UpdateTime"`
	}
	Asset struct {
		ID         string `json:"Id"`
		Name       string `json:"Name"`
		URL        string `json:"URL"`
		GroupID    string `json:"GroupId"`
		AssetType  string `json:"AssetType"`
		Status     string `json:"Status"`
		Moderation struct {
			Strategy string `json:"Strategy"`
		} `json:"Moderation,omitzero"`
		Error struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error,omitzero"`
		ProjectName       string    `json:"ProjectName,omitzero"`
		CreateTime        time.Time `json:"CreateTime,omitzero"`
		UpdateTime        time.Time `json:"UpdateTime,omitzero"`
		LastInferenceTime time.Time `json:"LastInferenceTime,omitzero"`
	}

	ListAssetGroupsRequest  = ListRequest
	ListAssetGroupsResponse = ListResponse[AssetGroup]

	ListAssetsRequest  = ListRequest
	ListAssetsResponse = ListResponse[Asset]
)

type (
	RequestWithID interface {
		Validate() error
		ResourceID() string
	}

	GetAssetGroupRequest struct {
		ID          string `json:"Id"`
		ProjectName string `json:"ProjectName,omitzero"`
	}
	GetAssetGroupResponse   = AssetGroup
	UpdateAssetGroupRequest struct {
		ID          string `json:"Id"`
		Name        string `json:"Name,omitzero"`
		Description string `json:"Description,omitzero"`
		ProjectName string `json:"ProjectName,omitzero"`
	}
	UpdateAssetGroupResponse = CreateAssetGroupResponse

	DeleteAssetGroupRequest  = GetAssetGroupRequest
	DeleteAssetGroupResponse = CreateAssetGroupResponse

	GetAssetRequest  = GetAssetGroupRequest
	GetAssetResponse = Asset

	UpdateAssetRequest struct {
		ID          string `json:"Id"`
		Name        string `json:"Name,omitzero"`
		ProjectName string `json:"ProjectName,omitzero"`
	}
	UpdateAssetResponse = CreateAssetResponse

	DeleteAssetRequest  = GetAssetRequest
	DeleteAssetResponse = CreateAssetGroupResponse
)

func (req GetAssetGroupRequest) Validate() error {
	if req.ID == "" {
		return errors.New("The required parameter Id is missing.")
	}
	return nil
}

func (req GetAssetGroupRequest) ResourceID() string { return req.ID }

func (req UpdateAssetGroupRequest) Validate() error {
	if req.ID == "" {
		return errors.New("The required parameter Id is missing.")
	}
	if req.Name != "" && utf8.RuneCountInString(req.Name) > 64 {
		return errors.New("Name must not exceed 64 characters")
	}
	if req.Description != "" && utf8.RuneCountInString(req.Description) > 300 {
		return errors.New("Description must not exceed 300 characters")
	}
	return nil
}

func (req UpdateAssetGroupRequest) ResourceID() string { return req.ID }

func (req UpdateAssetRequest) Validate() error {
	if req.ID == "" {
		return errors.New("The required parameter Id is missing.")
	}
	if utf8.RuneCountInString(req.Name) > 64 {
		return errors.New("Name must not exceed 64 characters")
	}
	return nil
}

func (req UpdateAssetRequest) ResourceID() string { return req.ID }

type (
	CreateVisualValidateSessionRequest struct {
		CallbackURL string `json:"CallbackURL,omitzero"`
		ProjectName string `json:"ProjectName,omitzero"`
	}
	CreateVisualValidateSessionResponse struct {
		BytedToken  string `json:"BytedToken"`
		H5Link      string `json:"H5Link"`
		CallbackURL string `json:"CallbackURL"`
	}

	GetVisualValidateResultRequest struct {
		BytedToken  string `json:"BytedToken"`
		ProjectName string `json:"ProjectName,omitzero"`
	}
	GetVisualValidateResultResponse struct {
		GroupID string `json:"GroupId"`
	}

	VisualValidateCallbackRequest struct {
		BytedToken            string `form:"bytedToken"`
		ResultCode            string `form:"resultCode"`
		AlgorithmBaseRespCode string `form:"algorithmBaseRespCode"`
		ReqMeasureInfoValue   string `form:"reqMeasureInfoValue"`
		VerifyType            string `form:"verify_type"`
	}
)
