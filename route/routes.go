package route

import (
	"loginTest/api"
	"loginTest/controller"
	"loginTest/middleware"

	"github.com/gin-gonic/gin"
)

// 创建路由

func CollectRoute(r *gin.Engine) *gin.Engine {

	r.Use(middleware.CORSMiddleware())
	// 这里把路由分成了三组，其中auth是需要token验证的，也就是需要用户登录
	// noauth是不需要token的，也就是不需要用户登录
	// oauth是OAuth带限制权限认证的，需要用户先授权
	auth := r.Group("")
	noauth := r.Group("")
	oauth := r.Group("")

	auth.Use(middleware.AuthMiddleware())
	oauth.Use(middleware.OAuthMiddleware())

	//OAuth2认证
	auth.POST("/api2/auth/connect/check", controller.CheckOAuth2App)
	auth.POST("/api2/auth/connect/authorize", controller.AuthorizeOAuth2App)
	noauth.POST("/api2/auth/connect/token", controller.GetTokenbyOAuth2)
	oauth.POST("/api2/auth/connect/userinfo", controller.GetInfobyOAuth2)
	//OAuth2授权记录管理
	auth.GET("/api2/auth/connect/records", controller.GetOAuth2AuthRecords)

	auth.POST("/api2/auth/submitRating", controller.UpdateRating) // 评分
	auth.POST("api2/auth/userPostRating", controller.GetUserPostRating)
	auth.POST("/api2/auth/getPostType", controller.GetPostType)           // 获取帖子类型
	auth.POST("/api2/auth/getAverageRating", controller.GetAverageRating) // 评分贴评分
	auth.POST("/api2/auth/getStars", controller.GetStars)
	auth.POST("api2/auth/postRcomment", controller.PostRcomment)

	noauth.POST("/api2/auth/register", controller.Register)
	noauth.POST("/api2/auth/login", controller.Login)
	auth.POST("/api2/auth/apiTest", api.ApiTest)
	auth.POST("/api2/auth/post", controller.Post)
	auth.POST("/api2/auth/browse", controller.Browse)
	auth.POST("/api2/auth/getPostNum", controller.GetPostNum)
	auth.POST("/api2/auth/deleteMe", controller.DeleteMe)
	auth.POST("/api2/auth/updateLike", controller.UpdateLike)
	auth.POST("/api2/auth/showDetails", controller.ShowDetails)
	auth.POST("api2/auth/showPcomments", controller.GetComments)
	auth.POST("api2/auth/postPcomment", controller.PostPcomment)
	auth.POST("api2/auth/postCcomment", controller.PostCcomment)
	auth.POST("api2/auth/updateCcommentLike", controller.UpdateCcommentLike)
	auth.POST("api2/auth/updatePcommentLike", controller.UpdatePcommentLike)
	auth.POST("api2/auth/updateCcommentDeny", controller.UpdateCcommentDeny)
	auth.POST("api2/auth/updatePcommentDeny", controller.UpdatePcommentDeny)
	auth.POST("/api2/auth/updateSave", controller.UpdateSave)
	auth.POST("/api2/auth/updateBrowseNum", controller.UpdateBrowseNum)
	auth.POST("/api2/auth/deletePost", controller.DeletePost)
	auth.POST("/api2/auth/deletePcomment", controller.DeletePcomment)
	auth.POST("/api2/auth/deleteCcomment", controller.DeleteCcomment)
	auth.POST("/api2/auth/submitReport", controller.SubmitReport)
	auth.GET("/api2/auth/info", controller.Info)
	noauth.POST("/api2/auth/uploadPhotos", controller.UploadPhotos)
	noauth.POST("/api2/auth/uploadAvatar", controller.UploadAvatar)
	noauth.POST("/api2/auth/updateAvatar", controller.UpdateAvatar)
	auth.POST("/api2/auth/getAvatar", controller.GetAvatar)
	auth.POST("/api2/auth/getInfo", controller.GetInfo)
	auth.POST("/api2/auth/updateUserInfo", controller.UpdateUserInfo)
	noauth.POST("/api2/auth/uploadZip", controller.UploadZip)
	auth.POST("/api2/auth/submitFeedback", controller.SubmitFeedback)
	auth.GET("/api2/auth/getAllFeedback", controller.GetAllFeedback)
	auth.GET("/api2/auth/calculateHeat", controller.CalculateHeat)
	auth.GET("/api2/auth/getNotice", controller.GetNotice)
	auth.GET("/api2/auth/getNoticeNum", controller.GetNoticeNum)
	auth.PATCH("api2/auth/readNotice/:noticeID", controller.ReadNotice)
	noauth.POST("/api2/auth/modifyPassword", controller.ModifyPassword)
	noauth.POST("/api2/auth/validateEmail", controller.ValidateEmail)
	auth.POST("/api2/auth/identityValidate", controller.IdentityValidate)
	auth.POST("/api2/auth/getFileList", controller.GetFileList)
	auth.POST("/api2/auth/fileDelete", controller.DeleteFile)
	auth.POST("/api2/auth/folderDelete", controller.DeleteFolder)
	auth.POST("/api2/auth/uploadFile", controller.TeachUpload)
	auth.POST("/api2/auth/addFolder", controller.AddFolder)
	auth.POST("/api2/auth/getObjectUrl", controller.GetObjectUrl)
	auth.POST("/api2/auth/changeEmailPush", controller.ChangeEmailPushStatus)
	auth.GET("/api2/auth/getTags", controller.GetTags)
	auth.POST("/api2/auth/gettitle", controller.GetTitle)
	//获得所有聊天用户列表，并且建立ws链接
	auth.GET("/websocket/auth/chat", controller.ChatHandler)
	//点击与某人聊天主页的接口，需要监听用户是否在聊天页面
	//auth.GET("/api2/auth/intoChatView", controller.IntoChatView)
	//退出与某人聊天主页的接口，需要监听用户是否在聊天页面
	//auth.POST("/api2/auth/leaveChatView", controller.LeaveChatView)
	auth.GET("/api2/auth/getChatHistory", controller.GetChatHistory)
	auth.GET("/api2/auth/getChatNotice", controller.GetChatNotice)
	//商城板块
	auth.POST("/api2/auth/postProduct", controller.PostProduct)
	auth.POST("/api2/auth/getProducts", controller.GetProducts)
	auth.POST("/api2/auth/getProductDetail", controller.GetProductDetail)
	auth.POST("/api2/auth/getProductNum", controller.GetProductNum)
	auth.POST("/api2/auth/deleteProduct", controller.DeleteProduct)
	auth.POST("/api2/auth/saleProduct", controller.SaleProduct)
	auth.POST("/api2/auth/getCarouselImg", controller.GetCarouselImg)

	// 给管理员设置一个新的路由分组
	adminAuth := r.Group("")
	adminAuth.Use(middleware.AuthMiddleware_admin())
	//adminAuth.POST("/api2/auth/passUsers", controller.PassUsers)
	adminAuth.POST("/api2/auth/addAdmin", controller.AddAdmin)
	adminAuth.POST("/api2/auth/changePassword", controller.ChangeAdminPassword)
	adminAuth.POST("/api2/auth/deleteUser", controller.DeleteUser)
	adminAuth.POST("/api2/auth/deleteAdmin", controller.DeleteAdmin)
	adminAuth.POST("/api2/auth/showUsers", controller.ShowFilterUsers)
	r.POST("/api2/auth/adminLogin", controller.AdminLogin)
	adminAuth.GET("/api2/auth/admininfo", controller.AdminInfo)
	adminAuth.GET("/api2/auth/getSues", controller.GetSues)
	adminAuth.POST("/api2/auth/noViolation", controller.NoViolation)
	adminAuth.POST("/api2/auth/violation", controller.Violation)
	adminAuth.POST("/api2/auth/adminPost", controller.AdminPost)
	return r
}
