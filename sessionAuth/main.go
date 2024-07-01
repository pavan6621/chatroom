package main

import (
	"log"
      "net/http"
	"example.com/my/module/database"
	"example.com/my/module/handlers"
	"example.com/my/module/middleware"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	// "github.com/gin-contrib/cors"

)
func main(){
	err:=database.ConnectDB()
	if err!=nil{
		log.Fatal("Failed to connect to database",err)
	}
	router:=gin.Default();
	store:=cookie.NewStore([]byte("secret"))
	
	router.Use(sessions.Sessions("mysession",store))
	
	// config := cors.DefaultConfig()
    // config.AllowOrigins = []string{"http://127.0.0.1:8080"} 
    // router.Use(cors.New(config))
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Content-Length, X-CSRF-Token, Token, session")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}
		c.Next()
	})


	router.POST("/register",handlers.RegisterHandler)
	router.POST("/login",handlers.LoginHandler)
	router.GET("/profile",middleware.AuthMiddleware(), handlers.ProfileHandler)
	router.GET("/logout", middleware.AuthMiddleware(), handlers.LogoutHandler)
	router.GET("/users", handlers.GetAllUsers)
	router.GET("/userspecific", handlers.GetSpecificUser)
	router.Run(":8080")

}