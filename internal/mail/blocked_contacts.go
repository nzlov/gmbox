package mail

import (
	"strings"

	"gmbox/internal/model"
)

// loadBlockedContactSet 预加载黑名单集合，避免同步过程中每封邮件都单独查库。
func (s *Service) loadBlockedContactSet() (map[string]struct{}, error) {
	var items []model.ContactBlacklist
	if err := s.db.Find(&items).Error; err != nil {
		return nil, err
	}
	blocked := make(map[string]struct{}, len(items))
	for _, item := range items {
		address := normalizeBlockedContactAddress(item.Address)
		if address == "" {
			continue
		}
		blocked[address] = struct{}{}
	}
	return blocked, nil
}

// normalizeBlockedContactAddress 统一发件人地址格式，避免大小写差异导致黑名单失效。
func normalizeBlockedContactAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

// isBlockedContact 判断当前发件人是否命中黑名单。
func isBlockedContact(address string, blocked map[string]struct{}) bool {
	if len(blocked) == 0 {
		return false
	}
	_, ok := blocked[normalizeBlockedContactAddress(address)]
	return ok
}
