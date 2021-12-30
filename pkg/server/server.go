package server

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "weave/docs"
	"weave/pkg/config"
	"weave/pkg/container"
	"weave/pkg/controller"
	"weave/pkg/database"
	"weave/pkg/middleware"
	"weave/pkg/repository"
	"weave/pkg/service"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

func New(conf *config.Config, logger *logrus.Logger) (*Server, error) {
	rateLimitMiddleware, err := middleware.RateLimitMiddleware(conf.Server.LimitConfigs)
	if err != nil {
		return nil, err
	}

	db, err := database.NewPostgres(&conf.DB)
	if err != nil {
		return nil, err
	}

	rdb, err := database.NewRedisClient(&conf.Redis)
	if err != nil {
		return nil, err
	}

	conClient, err := container.NewClient()
	if err != nil {
		logrus.Warningf("failed to create docker client, container api disabled: %v", err)
	}

	userRepository := repository.NewUserRepository(db, rdb)
	if err := userRepository.Migrate(); err != nil {
		return nil, err
	}

	userService := service.NewUserService(userRepository)
	jwtService := service.NewJWTService()

	userController := controller.NewUserController(userService)
	authContoller := controller.NewAuthController(userService, jwtService)
	containerController := controller.NewContainerController(conClient)

	gin.SetMode(conf.Server.ENV)

	e := gin.New()
	e.Use(
		rateLimitMiddleware,
		middleware.LogMiddleware(logger, "/api/v1"),
		middleware.MonitorMiddleware(),
		gin.Recovery(),
	)

	e.LoadHTMLFiles("web/terminal.html")

	return &Server{
		engine:              e,
		config:              conf,
		logger:              logger,
		userController:      userController,
		authContoller:       authContoller,
		containerController: containerController,
		authMiddleware:      middleware.AuthMiddleware(jwtService),
		db:                  db,
		rdb:                 rdb,
		containerClient:     conClient,
	}, nil
}

type Server struct {
	engine *gin.Engine
	config *config.Config
	logger *logrus.Logger

	userController      *controller.UserController
	authContoller       *controller.AuthController
	containerController *controller.ContainerController

	authMiddleware gin.HandlerFunc

	db              *gorm.DB
	rdb             *database.RedisDB
	containerClient *container.Client
}

// graceful shutdown
func (s *Server) Run() {
	defer s.Close()

	s.Routers()

	addr := fmt.Sprintf("%s:%d", s.config.Server.Address, s.config.Server.Port)
	s.logger.Infof("Start server on: %s", addr)

	server := &http.Server{
		Addr:    addr,
		Handler: s.engine,
	}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			s.logger.Fatalf("Failed to start server, %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.config.Server.GracefulShutdownPeriod)*time.Second)
	defer cancel()

	ch := <-sig
	s.logger.Infof("Receive signal: %s", ch)

	server.Shutdown(ctx)
}

func (s *Server) Close() {
	s.rdb.Close()
	db, _ := s.db.DB()
	if db != nil {
		db.Close()
	}
	if s.containerClient != nil {
		s.containerClient.Close()
	}
}

func (s *Server) Routers() {
	root := s.engine
	root.GET("/", s.Index)
	root.GET("/healthz", s.Healthz)
	root.GET("/metrics", gin.WrapH(promhttp.Handler()))
	root.Any("/debug/pprof/*any")
	root.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))

	root.POST("/login", s.authContoller.Login)
	root.GET("/logout", s.authContoller.Logout)
	root.POST("/register", s.authContoller.Register)

	api := root.Group("/api/v1")
	api.Use(s.authMiddleware)
	api.GET("/users", s.userController.List)
	api.POST("/users", s.userController.Create)
	api.GET("/users/:id", s.userController.Get)
	api.PUT("/users/:id", s.userController.Update)
	api.DELETE("/users/:id", s.userController.Delete)

	if s.containerClient != nil {
		api.GET("/containers", s.containerController.List)
		api.POST("/containers", s.containerController.Create)
		api.GET("/containers/:id", s.containerController.Get)
		api.PUT("/containers/:id", s.containerController.Update)
		api.POST("/containers/:id", s.containerController.Operate)
		api.DELETE("/containers/:id", s.containerController.Delete)
		api.GET("/containers/:id/log", s.containerController.Log)
		api.GET("/containers/:id/exec", s.containerController.Exec)
		api.GET("/containers/:id/terminal", func(c *gin.Context) {
			c.HTML(200, "terminal.html", nil)
		})
	}
}

// @Summary Index
// @Produce html
// @Tags index
// @Router / [get]
// @Success 200 {string}  string    ""
func (s *Server) Index(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(
		`<html>
	<head>
		<title>Weave Server</title>
	</head>
	<body>
		<h1>Hello Weave</h1>
		<ul>
			<li><a href="/swagger/index.html">swagger</a></li>
			<li><a href="/metrics">metrics</a></li>
			<li><a href="/healthz">healthz</a></li>
	  	</ul>
		<hr>
		<center>Weave/1.0</center>
	</body>
<html>`))
}

// @Summary Healthz
// @Produce json
// @Tags healthz
// @Success 200 {string}  string    "ok"
// @Router /healthz [get]
func (s *Server) Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, "ok")
}
