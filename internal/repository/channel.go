// internal/repository/channel.go
package repository

import (
	"fmt"
	"strings"

	"github.com/example/epay-go/internal/database"
	"github.com/example/epay-go/internal/model"
	"gorm.io/gorm"
)

type ChannelRepository struct {
	db *gorm.DB
}

func NewChannelRepository() *ChannelRepository {
	return &ChannelRepository{db: database.Get()}
}

// Create 创建通道
func (r *ChannelRepository) Create(channel *model.Channel) error {
	return r.db.Create(channel).Error
}

// GetByID 根据ID获取通道
func (r *ChannelRepository) GetByID(id int64) (*model.Channel, error) {
	var channel model.Channel
	err := r.db.First(&channel, id).Error
	if err != nil {
		return nil, err
	}
	return &channel, nil
}

// Update 更新通道
func (r *ChannelRepository) Update(channel *model.Channel) error {
	return r.db.Save(channel).Error
}

// Delete 删除通道
func (r *ChannelRepository) Delete(id int64) error {
	return r.db.Delete(&model.Channel{}, id).Error
}

// List 分页查询通道列表
func (r *ChannelRepository) List(page, pageSize int) ([]model.Channel, int64, error) {
	var channels []model.Channel
	var total int64

	err := r.db.Model(&model.Channel{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = r.db.Offset(offset).Limit(pageSize).Order("sort ASC, id ASC").Find(&channels).Error
	if err != nil {
		return nil, 0, err
	}

	return channels, total, nil
}

// ListEnabled 获取所有启用的通道
func (r *ChannelRepository) ListEnabled() ([]model.Channel, error) {
	var channels []model.Channel
	err := r.db.Where("status = ?", 1).Order("sort ASC, id ASC").Find(&channels).Error
	return channels, err
}

// GetByPluginAndPayType 根据插件和支付类型获取可用通道
func (r *ChannelRepository) GetByPluginAndPayType(plugin, payType string) (*model.Channel, error) {
	var channel model.Channel
	query := r.db.Where("plugin = ? AND status = 1", plugin)
	if strings.TrimSpace(payType) != "" {
		query = query.Where("pay_types LIKE ?", "%"+payType+"%")
	}
	err := query.Order("sort ASC").First(&channel).Error
	if err != nil {
		return nil, err
	}
	return &channel, nil
}

// GetAvailableByPayType 根据支付类型获取可用通道（不考虑 payMethod）
func (r *ChannelRepository) GetAvailableByPayType(payType string) (*model.Channel, error) {
	return r.GetAvailableByPayTypeAndMethod(payType, "")
}

// expandMethodAliases 展开支付方式的等价别名
// type_normalizer 会把 scan/qrcode/precreate 统一为 native，
// 但 DB 中 pay_types 可能存储 scan（如汇付），所以需要展开所有别名进行匹配
func expandMethodAliases(method string) []string {
	switch method {
	case "native", "scan", "qrcode", "precreate":
		return []string{"native", "scan", "qrcode", "precreate"}
	case "h5", "wap":
		return []string{"h5", "wap"}
	case "web", "pc", "page":
		return []string{"web", "pc", "page"}
	default:
		return []string{method}
	}
}

// buildMethodLikeConditions 构造 pay_types 字段对多个别名的 OR 匹配条件
func buildMethodLikeConditions(aliases []string) (string, []interface{}) {
	parts := make([]string, len(aliases))
	args := make([]interface{}, len(aliases))
	for i, alias := range aliases {
		parts[i] = "pay_types LIKE ?"
		args[i] = "%" + alias + "%"
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}

// GetAvailableByPayTypeAndMethod 根据支付类型和支付方式获取可用通道
// payType: wxpay/alipay 等渠道标识
// payMethod: scan/jsapi/h5/native 等具体支付方式（可为空）
func (r *ChannelRepository) GetAvailableByPayTypeAndMethod(payType, payMethod string) (*model.Channel, error) {
	var channel model.Channel
	normalizedPayType := strings.ToLower(strings.TrimSpace(payType))
	normalizedMethod := strings.ToLower(strings.TrimSpace(payMethod))

	// 展开支付方式别名（native ↔ scan 等）
	var methodAliases []string
	if normalizedMethod != "" {
		methodAliases = expandMethodAliases(normalizedMethod)
	}

	query := r.db.Where("status = 1")

	// 根据 payType 筛选允许的 plugin 
	switch normalizedPayType {
	case "wxpay", "wechat":
		query = query.Where("plugin IN (?) OR plugin LIKE ?", []string{"wechat", "hf-wxpay"}, "%wechat%")
	case "alipay":
		query = query.Where("plugin IN (?) OR plugin LIKE ?", []string{"alipay", "hf-alipay"}, "%alipay%")
	default:
		// 未知渠道类型，直接按 pay_types 模糊匹配
		query = query.Where("pay_types LIKE ?", "%"+normalizedPayType+"%")
	}

	// 如果有 payMethod，还需要确认通道支持该方式
	if len(methodAliases) > 0 {
		cond, args := buildMethodLikeConditions(methodAliases)
		query = query.Where(cond, args...)
	}

	err := query.Order("sort ASC").First(&channel).Error
	if err == nil {
		return &channel, nil
	}

	return nil, fmt.Errorf("no available channel for pay_type=%s pay_method=%s", payType, payMethod)
}
