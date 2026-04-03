package httpapi

import (
	"fmt"
	"io/fs"
	"net/http"
	"strconv"

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
		query := app.DB.Order("sent_at desc, id desc")
		if accountID := c.Query("account_id"); accountID != "" {
			query = query.Where("account_id = ?", accountID)
		}
		if err := query.Limit(100).Find(&messages).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询邮件失败"})
			return
		}
		c.JSON(http.StatusOK, messages)
	})

	protected.GET("/messages/:id", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "邮件 ID 不合法"})
			return
		}
		var message model.Message
		if err := app.DB.First(&message, id).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"message": "邮件不存在"})
			return
		}
		var body model.MessageBody
		if err := app.DB.Where("message_id = ?", message.Model.ID).First(&body).Error; err != nil && err != gorm.ErrRecordNotFound {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "查询邮件正文失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": message, "body": body})
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
		c.JSON(http.StatusNotFound, gin.H{"message": "邮箱不存在"})
		return nil, false
	}
	return &account, true
}
