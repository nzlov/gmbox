package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"gmbox/internal/auth"
	"gmbox/internal/mail"
	"gmbox/internal/model"
	"gmbox/internal/runtime"
)

// NewRouter 组装 API、鉴权中间件和前端静态资源路由。
func NewRouter(app *runtime.App, assets fs.FS) *gin.Engine {
	router := gin.Default()

	api := router.Group("/api")
	registerPublic(api, app)
	registerProtected(api, app)

	registerFrontend(router, assets)
	return router
}

// registerPublic 注册登录等无需鉴权的接口。
func registerPublic(api *gin.RouterGroup, app *runtime.App) {
	authGroup := api.Group("/auth")
	authGroup.POST("/login", func(c *gin.Context) {
		var req struct {
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		var user model.User
		if err := app.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"message": "用户名或密码错误"})
			return
		}
		if !auth.ComparePassword(user.PasswordHash, req.Password) {
			c.JSON(http.StatusUnauthorized, gin.H{"message": "用户名或密码错误"})
			return
		}
		token, err := app.JWT.Sign(user.Model.ID, user.Username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "签发令牌失败"})
			return
		}
		c.SetCookie(app.Config.Auth.CookieName, token, int(app.Config.JWTExpireDuration().Seconds()), "/", "", false, true)
		c.JSON(http.StatusOK, gin.H{"username": user.Username})
	})
	api.GET("/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"name": app.Config.App.Name, "status": "ok"})
	})
}

// registerProtected 注册需要登录后才能访问的核心业务接口。
func registerProtected(api *gin.RouterGroup, app *runtime.App) {
	protected := api.Group("")
	protected.Use(auth.Middleware(app.Config.Auth.CookieName, app.JWT))

	protected.GET("/auth/me", func(c *gin.Context) {
		claims := auth.MustClaims(c)
		c.JSON(http.StatusOK, gin.H{"user_id": claims.UserID, "username": claims.Username})
	})
	protected.POST("/auth/logout", func(c *gin.Context) {
		c.SetCookie(app.Config.Auth.CookieName, "", -1, "/", "", false, true)
		c.JSON(http.StatusOK, gin.H{"message": "已退出登录"})
	})
	protected.GET("/account-providers", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"items":                   mail.ProviderPresets(),
			"microsoft_oauth_enabled": app.Mailer.MicrosoftOAuthEnabled(),
		})
	})
	protected.GET("/accounts/oauth/microsoft/config", func(c *gin.Context) {
		tenantID := strings.TrimSpace(app.Config.MicrosoftOAuth.TenantID)
		if tenantID == "" {
			tenantID = "common"
		}
		redirectURL := resolveMicrosoftOAuthFrontendRedirectURL(c, app)
		c.JSON(http.StatusOK, gin.H{
			"enabled":      app.Mailer.MicrosoftOAuthEnabled(),
			"client_id":    app.Config.MicrosoftOAuth.ClientID,
			"tenant_id":    tenantID,
			"redirect_uri": redirectURL,
			"scope":        mail.MicrosoftOAuthScope(),
			"flow":         microsoftOAuthFlow(redirectURL),
		})
	})

	protected.GET("/accounts", func(c *gin.Context) {
		var accounts []model.MailAccount
		if err := app.DB.Order("id desc").Find(&accounts).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询邮箱失败"})
			return
		}
		c.JSON(http.StatusOK, accounts)
	})

	protected.POST("/accounts", func(c *gin.Context) {
		var input mail.AccountInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		account := &model.MailAccount{}
		if err := app.Mailer.SaveAccount(account, input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, account)
	})

	protected.POST("/accounts/import", func(c *gin.Context) {
		var req struct {
			Items []mail.AccountInput `json:"items" binding:"required,min=1"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "导入参数不合法"})
			return
		}
		accounts := make([]model.MailAccount, 0, len(req.Items))
		if err := app.DB.Transaction(func(tx *gorm.DB) error {
			mailer := app.Mailer.WithDB(tx)
			for index, item := range req.Items {
				if strings.EqualFold(strings.TrimSpace(item.AuthType), "oauth") {
					return fmt.Errorf("第 %d 条记录使用了 OAuth，批量导入仅支持非 OAuth 邮箱", index+1)
				}
				account := &model.MailAccount{}
				if err := mailer.SaveAccount(account, item); err != nil {
					return fmt.Errorf("第 %d 条记录导入失败: %s", index+1, err.Error())
				}
				accounts = append(accounts, *account)
			}
			return nil
		}); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": accounts, "message": fmt.Sprintf("成功导入 %d 个邮箱", len(accounts))})
	})

	protected.GET("/accounts/oauth/microsoft/start", func(c *gin.Context) {
		state, err := mail.CreateOAuthState()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "生成 OAuth state 失败"})
			return
		}
		loginHint := strings.TrimSpace(c.Query("login_hint"))
		redirectURL, err := app.Mailer.BuildMicrosoftPKCEOAuthURL(state, resolveMicrosoftOAuthLegacyRedirectURL(c, app), "", loginHint)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.SetCookie("gmbox_oauth_state", state, 600, "/", "", false, true)
		if c.Query("popup") == "1" {
			c.SetCookie("gmbox_oauth_popup", "1", 600, "/", "", false, true)
		} else {
			c.SetCookie("gmbox_oauth_popup", "", -1, "/", "", false, true)
		}
		c.Redirect(http.StatusFound, redirectURL)
	})

	protected.GET("/accounts/oauth/microsoft/callback", func(c *gin.Context) {
		popup := isOAuthPopup(c)
		state, err := c.Cookie("gmbox_oauth_state")
		if err != nil || state == "" || state != c.Query("state") {
			respondOAuthResult(c, popup, false, "微软 OAuth state 校验失败，请重试")
			return
		}
		c.SetCookie("gmbox_oauth_state", "", -1, "/", "", false, true)
		c.SetCookie("gmbox_oauth_popup", "", -1, "/", "", false, true)
		if queryErr := strings.TrimSpace(c.Query("error")); queryErr != "" {
			respondOAuthResult(c, popup, false, queryErr)
			return
		}
		code := strings.TrimSpace(c.Query("code"))
		if code == "" {
			respondOAuthResult(c, popup, false, "微软 OAuth 未返回授权码")
			return
		}
		account, err := app.Mailer.UpsertMicrosoftOAuthAccount(c.Request.Context(), code, resolveMicrosoftOAuthLegacyRedirectURL(c, app))
		if err != nil {
			respondOAuthResult(c, popup, false, err.Error())
			return
		}
		respondOAuthResult(c, popup, true, fmt.Sprintf("微软 OAuth 登录成功，已接入邮箱 %s", account.Email))
	})
	protected.POST("/accounts/oauth/microsoft/exchange", func(c *gin.Context) {
		var req struct {
			Code         string `json:"code" binding:"required"`
			CodeVerifier string `json:"code_verifier" binding:"required"`
			State        string `json:"state" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		account, err := app.Mailer.UpsertMicrosoftOAuthAccountWithPKCE(
			c.Request.Context(),
			strings.TrimSpace(req.Code),
			strings.TrimSpace(req.CodeVerifier),
			resolveMicrosoftOAuthFrontendRedirectURL(c, app),
		)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message": fmt.Sprintf("微软 OAuth 登录成功，已接入邮箱 %s", account.Email),
			"account": account,
		})
	})

	protected.PUT("/accounts/:id", func(c *gin.Context) {
		account, ok := loadAccount(c, app.DB)
		if !ok {
			return
		}
		var input mail.AccountInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		if err := app.Mailer.SaveAccount(account, input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, account)
	})

	protected.DELETE("/accounts/:id", func(c *gin.Context) {
		account, ok := loadAccount(c, app.DB)
		if !ok {
			return
		}
		if err := app.DB.Delete(account).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "删除邮箱失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
	})

	protected.POST("/accounts/:id/test", func(c *gin.Context) {
		account, ok := loadAccount(c, app.DB)
		if !ok {
			return
		}
		if err := app.Mailer.TestConnection(c.Request.Context(), *account); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "连接测试成功"})
	})

	protected.POST("/accounts/:id/sync", func(c *gin.Context) {
		account, ok := loadAccount(c, app.DB)
		if !ok {
			return
		}
		if err := app.Syncer.RunAccountNow(c.Request.Context(), *account); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "同步已完成"})
	})

	protected.GET("/messages", func(c *gin.Context) {
		var messages []model.Message
		query := app.DB.Model(&model.Message{}).Where("is_deleted = ?", false)
		if accountID := c.Query("account_id"); accountID != "" {
			query = query.Where("account_id = ?", accountID)
		}
		if folder := c.Query("folder"); folder != "" {
			query = query.Where("folder = ?", folder)
		}
		if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
			like := "%" + keyword + "%"
			query = query.Where(
				"subject LIKE ? OR from_name LIKE ? OR from_address LIKE ? OR snippet LIKE ?",
				like,
				like,
				like,
				like,
			)
		}

		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		if page < 1 {
			page = 1
		}
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(app.Config.Mail.PageSize)))
		if pageSize < 1 {
			pageSize = app.Config.Mail.PageSize
		}
		if pageSize > 200 {
			pageSize = 200
		}

		var total int64
		if err := query.Count(&total).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "统计邮件失败"})
			return
		}
		if err := query.Order("sent_at desc, id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&messages).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询邮件失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"items":     messages,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		})
	})

	protected.GET("/mailboxes", func(c *gin.Context) {
		accountID, _ := strconv.Atoi(c.DefaultQuery("account_id", "0"))
		mailboxes, err := app.Mailer.ListMailboxes(uint(accountID))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询文件夹失败"})
			return
		}
		c.JSON(http.StatusOK, mailboxes)
	})

	protected.GET("/messages/:id", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "邮件 ID 不合法"})
			return
		}
		message, body, attachments, err := app.Mailer.GetMessageDetail(uint(id))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"message": "邮件不存在"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": message, "body": body, "attachments": attachments})
	})

	protected.POST("/messages/send", func(c *gin.Context) {
		var input mail.SendInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求参数不合法"})
			return
		}
		if err := app.Mailer.Send(c.Request.Context(), input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "发送成功"})
	})

	protected.POST("/messages/:id/read", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "邮件 ID 不合法"})
			return
		}
		if err := app.Mailer.SetMessageRead(c.Request.Context(), uint(id), true); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "已标记为已读"})
	})

	protected.POST("/messages/:id/unread", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "邮件 ID 不合法"})
			return
		}
		if err := app.Mailer.SetMessageRead(c.Request.Context(), uint(id), false); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "已标记为未读"})
	})

	protected.POST("/messages/:id/delete", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "邮件 ID 不合法"})
			return
		}
		if err := app.Mailer.DeleteMessage(c.Request.Context(), uint(id)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
	})

	protected.POST("/messages/:id/move", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "邮件 ID 不合法"})
			return
		}
		var req struct {
			Folder string `json:"folder" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "目标文件夹不能为空"})
			return
		}
		if err := app.Mailer.MoveMessage(c.Request.Context(), uint(id), req.Folder); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "移动成功"})
	})

	protected.GET("/attachments/:id/download", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "附件 ID 不合法"})
			return
		}
		attachment, content, err := app.Mailer.DownloadAttachment(uint(id))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"message": "附件不存在"})
			return
		}
		contentType := attachment.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", attachment.FileName))
		c.Data(http.StatusOK, contentType, content)
	})

	protected.GET("/sync-states", func(c *gin.Context) {
		var states []model.SyncState
		if err := app.DB.Order("id desc").Find(&states).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询同步状态失败"})
			return
		}
		c.JSON(http.StatusOK, states)
	})
}

// registerFrontend 让非 API 请求统一交给前端路由处理。
func registerFrontend(router *gin.Engine, assets fs.FS) {
	fileServer := http.FileServer(http.FS(assets))
	router.NoRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet || len(c.Request.URL.Path) >= 5 && c.Request.URL.Path[:5] == "/api/" {
			c.JSON(http.StatusNotFound, gin.H{"message": "未找到资源"})
			return
		}
		if _, err := fs.Stat(assets, c.Request.URL.Path[1:]); err == nil && c.Request.URL.Path != "/" {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}
		index, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, fmt.Sprintf("读取前端入口失败: %v", err))
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})
	router.StaticFS("/assets", http.FS(assets))
}

// loadAccount 统一解析邮箱 ID 并查询数据库，减少重复分支。
func loadAccount(c *gin.Context, db *gorm.DB) (*model.MailAccount, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "邮箱 ID 不合法"})
		return nil, false
	}
	var account model.MailAccount
	if err := db.First(&account, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"message": "邮箱不存在"})
			return nil, false
		}
		c.JSON(http.StatusNotFound, gin.H{"message": "邮箱不存在"})
		return nil, false
	}
	return &account, true
}

// redirectAccounts 统一把 OAuth 结果带回邮箱管理页，避免前端再额外处理回调页。
func redirectAccounts(c *gin.Context, key string, value string) {
	query := fmt.Sprintf("/accounts?%s=%s", key, url.QueryEscape(value))
	c.Redirect(http.StatusFound, query)
}

// resolveMicrosoftOAuthFrontendRedirectURL 优先使用显式配置，未配置时按当前访问入口动态推导前端回调地址。
func resolveMicrosoftOAuthFrontendRedirectURL(c *gin.Context, app *runtime.App) string {
	configured := strings.TrimSpace(app.Config.MicrosoftOAuth.RedirectURL)
	if configured != "" {
		return configured
	}
	return requestBaseURL(c) + "/oauth/microsoft/callback"
}

// resolveMicrosoftOAuthLegacyRedirectURL 优先使用显式配置的旧 API 回调地址，未配置时再按当前访问入口推导。
func resolveMicrosoftOAuthLegacyRedirectURL(c *gin.Context, app *runtime.App) string {
	configured := strings.TrimSpace(app.Config.MicrosoftOAuth.RedirectURL)
	if configured != "" {
		return configured
	}
	return requestBaseURL(c) + "/api/accounts/oauth/microsoft/callback"
}

// microsoftOAuthFlow 根据回调地址判断当前应使用 PKCE 还是旧服务端兼容流。
func microsoftOAuthFlow(redirectURL string) string {
	if strings.HasSuffix(strings.TrimSpace(redirectURL), "/api/accounts/oauth/microsoft/callback") {
		return "legacy"
	}
	return "pkce"
}

// isOAuthPopup 判断旧服务端流是否由弹窗发起，以便回调时选择 postMessage 还是页面重定向。
func isOAuthPopup(c *gin.Context) bool {
	value, _ := c.Cookie("gmbox_oauth_popup")
	return strings.TrimSpace(value) == "1"
}

// respondOAuthResult 统一兼容弹窗回调与页面回跳两种结果返回方式。
func respondOAuthResult(c *gin.Context, popup bool, success bool, message string) {
	if !popup {
		if success {
			redirectAccounts(c, "oauth_success", message)
			return
		}
		redirectAccounts(c, "oauth_error", message)
		return
	}
	payload, err := json.Marshal(gin.H{
		"type":    "microsoft-oauth",
		"success": success,
		"message": message,
	})
	if err != nil {
		c.String(http.StatusInternalServerError, "OAuth 结果序列化失败")
		return
	}
	html := fmt.Sprintf(`<!doctype html><html><body><script>
window.opener && window.opener.postMessage(%s, window.location.origin);
window.close();
</script></body></html>`, payload)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// requestBaseURL 优先尊重反向代理透传的协议和主机，避免 OAuth 回调地址落成内网地址。
func requestBaseURL(c *gin.Context) string {
	scheme := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = c.Request.Host
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}
