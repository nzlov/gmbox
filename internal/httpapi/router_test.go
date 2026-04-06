package httpapi

import (
	"fmt"
	"net/url"
	"reflect"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"gmbox/internal/model"
)

// TestRedactQueryForLog 确保调试日志不会泄漏 OAuth 回调里的授权码和 PKCE 凭证。
func TestRedactQueryForLog(t *testing.T) {
	target, err := url.Parse("https://example.com/oauth/microsoft/callback?code=secret-code&state=secret-state&code_verifier=secret-verifier&plain=value")
	if err != nil {
		t.Fatalf("解析 URL 失败: %v", err)
	}
	redacted := redactQueryForLog(target)
	values, err := url.ParseQuery(redacted)
	if err != nil {
		t.Fatalf("解析脱敏后的 query 失败: %v", err)
	}
	if values.Get("code") != "<redacted>" {
		t.Fatalf("code = %q, want <redacted>", values.Get("code"))
	}
	if values.Get("state") != "<redacted>" {
		t.Fatalf("state = %q, want <redacted>", values.Get("state"))
	}
	if values.Get("code_verifier") != "<redacted>" {
		t.Fatalf("code_verifier = %q, want <redacted>", values.Get("code_verifier"))
	}
	if values.Get("plain") != "value" {
		t.Fatalf("plain = %q, want value", values.Get("plain"))
	}
}

// TestGroupContacts 确保联系人聚合后列表统计、主联系人和成员清单都会按同一组关系折叠。
func TestGroupContacts(t *testing.T) {
	groups := groupContacts([]contactSummary{
		{Address: "boss@example.com", Name: "老板", LatestSentAt: contactTimestamp("2026-04-06T10:00:00Z"), Total: 2},
		{Address: "ceo@example.com", Name: "CEO", LatestSentAt: contactTimestamp("2026-04-06T11:00:00Z"), Total: 3},
		{Address: "solo@example.com", Name: "独立联系人", LatestSentAt: contactTimestamp("2026-04-05T09:00:00Z"), Total: 1},
	}, map[string]string{
		"ceo@example.com": "boss@example.com",
	})

	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(groups))
	}
	if groups[0].Address != "boss@example.com" {
		t.Fatalf("groups[0].address = %q, want boss@example.com", groups[0].Address)
	}
	if groups[0].Total != 5 {
		t.Fatalf("groups[0].total = %d, want 5", groups[0].Total)
	}
	if groups[0].MemberCount != 2 {
		t.Fatalf("groups[0].member_count = %d, want 2", groups[0].MemberCount)
	}
	wantMembers := []contactMember{{Address: "boss@example.com", Name: "老板"}, {Address: "ceo@example.com", Name: "CEO"}}
	if !reflect.DeepEqual(groups[0].Members, wantMembers) {
		t.Fatalf("groups[0].members = %#v, want %#v", groups[0].Members, wantMembers)
	}
	if groups[0].Name != "老板" {
		t.Fatalf("groups[0].name = %q, want 老板", groups[0].Name)
	}
	if groups[0].LatestSentAt != contactTimestamp("2026-04-06T11:00:00Z") {
		t.Fatalf("groups[0].latest_sent_at = %q, want 2026-04-06T11:00:00Z", groups[0].LatestSentAt)
	}
	if groups[1].Address != "solo@example.com" {
		t.Fatalf("groups[1].address = %q, want solo@example.com", groups[1].Address)
	}
}

// TestGroupContactsKeepMappedOnlyMember 确保手工加入但尚无邮件的邮箱仍会显示在聚合成员里，避免刷新后丢失。
func TestGroupContactsKeepMappedOnlyMember(t *testing.T) {
	groups := groupContacts([]contactSummary{
		{Address: "boss@example.com", Name: "老板", LatestSentAt: contactTimestamp("2026-04-06T10:00:00Z"), Total: 2},
	}, map[string]string{
		"alias@example.com": "boss@example.com",
	})
	if len(groups) != 1 {
		t.Fatalf("len(groups) = %d, want 1", len(groups))
	}
	wantMembers := []contactMember{{Address: "boss@example.com", Name: "老板"}, {Address: "alias@example.com", Name: ""}}
	if !reflect.DeepEqual(groups[0].Members, wantMembers) {
		t.Fatalf("groups[0].members = %#v, want %#v", groups[0].Members, wantMembers)
	}
	if groups[0].MemberCount != 2 {
		t.Fatalf("groups[0].member_count = %d, want 2", groups[0].MemberCount)
	}
}

// TestApplyContactBlockedState 确保仅在整组成员都被拉黑时才把联系人组标记为黑名单状态。
func TestApplyContactBlockedState(t *testing.T) {
	groups := []contactItem{
		{Address: "boss@example.com", Members: []contactMember{{Address: "boss@example.com"}, {Address: "ceo@example.com"}}},
		{Address: "solo@example.com", Members: []contactMember{{Address: "solo@example.com"}}},
	}
	applyContactBlockedState(groups, map[string]struct{}{
		"boss@example.com": {},
		"ceo@example.com":  {},
	})
	if !groups[0].IsBlocked {
		t.Fatal("groups[0].is_blocked = false, want true")
	}
	if groups[1].IsBlocked {
		t.Fatal("groups[1].is_blocked = true, want false")
	}
}

// TestLoadContactGroupsKeepsBlacklistOnlyContact 确保仅存在于黑名单中的联系人仍会显示在列表里，便于后续解除拉黑。
func TestLoadContactGroupsKeepsBlacklistOnlyContact(t *testing.T) {
	db := openContactTestDB(t)
	if err := db.Create(&model.ContactBlacklist{Address: "blocked@example.com"}).Error; err != nil {
		t.Fatalf("写入黑名单失败: %v", err)
	}

	groups, err := loadContactGroups(db, "")
	if err != nil {
		t.Fatalf("loadContactGroups() error = %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("len(groups) = %d, want 1", len(groups))
	}
	if groups[0].Address != "blocked@example.com" {
		t.Fatalf("groups[0].address = %q, want blocked@example.com", groups[0].Address)
	}
	if !groups[0].IsBlocked {
		t.Fatal("groups[0].is_blocked = false, want true")
	}
	if groups[0].MemberCount != 1 {
		t.Fatalf("groups[0].member_count = %d, want 1", groups[0].MemberCount)
	}
}

// TestExpandContactGroupMembers 确保从任意成员进入时都能展开同一聚合组全部地址。
func TestExpandContactGroupMembers(t *testing.T) {
	got := expandContactGroupMembers(
		[]string{"ceo@example.com"},
		map[string]string{"ceo@example.com": "boss@example.com", "cto@example.com": "boss@example.com"},
		[]string{"boss@example.com", "ceo@example.com", "cto@example.com", "solo@example.com"},
	)
	want := []string{"boss@example.com", "ceo@example.com", "cto@example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expandContactGroupMembers() = %#v, want %#v", got, want)
	}
}

// TestFilterContactMessages 确保联系人邮件筛选会命中聚合成员的发件和收件关系。
func TestFilterContactMessages(t *testing.T) {
	messages := []model.Message{
		{FromAddress: "boss@example.com", ToAddresses: "Team <team@example.com>"},
		{FromAddress: "other@example.com", ToAddresses: "Boss <boss@example.com>"},
		{FromAddress: "other@example.com", ToAddresses: "Nobody <nobody@example.com>"},
	}
	filtered := filterContactMessages(messages, []string{"boss@example.com", "ceo@example.com"})
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
}

// TestCanonicalContactAddress 确保手工输入邮箱时会先做格式校验，避免无效地址写入聚合映射。
func TestCanonicalContactAddress(t *testing.T) {
	got, err := canonicalContactAddress(" Boss <Boss@Example.com> ")
	if err != nil {
		t.Fatalf("canonicalContactAddress() error = %v", err)
	}
	if got != "boss@example.com" {
		t.Fatalf("canonicalContactAddress() = %q, want boss@example.com", got)
	}
	if _, err := canonicalContactAddress("not-an-email"); err == nil {
		t.Fatal("canonicalContactAddress() expected validation error")
	}
}

// TestUpdateContactAggregationKeepsOriginalOnValidationError 确保更新聚合时若新成员校验失败，不会把原聚合关系拆掉。
func TestUpdateContactAggregationKeepsOriginalOnValidationError(t *testing.T) {
	db := openContactTestDB(t)
	seed := []model.ContactAggregation{
		{Address: "ceo@example.com", PrimaryAddress: "boss@example.com"},
		{Address: "cto@example.com", PrimaryAddress: "boss@example.com"},
	}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatalf("写入测试聚合关系失败: %v", err)
	}

	err := updateContactAggregation(db, "boss@example.com", "boss@example.com", []string{"boss@example.com", "not-an-email"})
	if err == nil {
		t.Fatal("updateContactAggregation() expected validation error")
	}

	var items []model.ContactAggregation
	if err := db.Order("address asc").Find(&items).Error; err != nil {
		t.Fatalf("读取测试聚合关系失败: %v", err)
	}
	want := []model.ContactAggregation{
		{Address: "ceo@example.com", PrimaryAddress: "boss@example.com"},
		{Address: "cto@example.com", PrimaryAddress: "boss@example.com"},
	}
	if len(items) != len(want) {
		t.Fatalf("len(items) = %d, want %d", len(items), len(want))
	}
	for index := range want {
		if items[index].Address != want[index].Address || items[index].PrimaryAddress != want[index].PrimaryAddress {
			t.Fatalf("items[%d] = %#v, want %#v", index, items[index], want[index])
		}
	}
}

// TestBlockContactsExpandsAggregation 确保拉黑主联系人时会把整组成员一起加入黑名单，避免聚合联系人漏拦截。
func TestBlockContactsExpandsAggregation(t *testing.T) {
	db := openContactTestDB(t)
	seed := []model.ContactAggregation{{Address: "ceo@example.com", PrimaryAddress: "boss@example.com"}}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatalf("写入测试聚合关系失败: %v", err)
	}

	if err := blockContacts(db, []string{"boss@example.com"}); err != nil {
		t.Fatalf("blockContacts() error = %v", err)
	}

	var items []model.ContactBlacklist
	if err := db.Order("address asc").Find(&items).Error; err != nil {
		t.Fatalf("读取黑名单失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Address != "boss@example.com" || items[1].Address != "ceo@example.com" {
		t.Fatalf("blacklist addresses = %#v, want boss/ceo", []string{items[0].Address, items[1].Address})
	}
}

func openContactTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&model.ContactAggregation{}, &model.ContactBlacklist{}, &model.Message{}, &model.MailAccount{}); err != nil {
		t.Fatalf("迁移测试数据库失败: %v", err)
	}
	return db
}
