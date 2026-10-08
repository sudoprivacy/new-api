// sudoapi: API for sudowork

package model

type QueryUserTokenOptions struct {
	Paginator

	ID     int `json:"id" form:"id"`
	UserID int `json:"user_id" form:"user_id"`
}

func QueryUserTokens(opt QueryUserTokenOptions) ([]*Token, int64, error) {
	query := DB.Model(&Token{}).Where("user_id = ?", opt.UserID)
	if opt.ID != 0 {
		query = query.Where("id = ?", opt.ID)
	}
	query = query.Order("id desc")
	return FindByPaginator[Token](query, opt.Paginator)
}
