package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/dzovi/leave-management/assets"
	"github.com/dzovi/leave-management/store"
	"github.com/dzovi/leave-management/views"
	"github.com/gin-gonic/gin"
)

func now() string {
	return time.Now().Format(time.RFC3339)
}

// dbURL reads DATABASE_URL, falling back to the local Docker Postgres +
// dedicated leave_management database created for this project.
func dbURL() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://postgres:postgres@localhost:5432/leave_management?sslmode=disable"
}

// html sets the headers + renders a templ component, the pattern we've used
// since Phase 2. Factored out here now that we have several handlers.
func html(c *gin.Context, render func() error) {
	c.Status(200)
	c.Header("Content-Type", "text/html; charset=utf-8")
	_ = render()
}

func main() {
	ctx := context.Background()

	db, err := store.New(ctx, dbURL())
	if err != nil {
		log.Fatalf("connect to postgres: %v", err)
	}
	defer db.Close()

	// Phase 8: dev mode (VITE_DEV=true) points the layout at the Vite dev server
	// for HMR; otherwise we read the built manifest to emit hashed asset tags.
	if err := assets.Init(os.Getenv("VITE_DEV") == "true"); err != nil {
		log.Fatalf("load asset manifest (did you run `npm run build` in web/?): %v", err)
	}

	r := gin.Default()

	// Serve the Vite build output. Unused in dev (assets come from :5173).
	r.Static("/build", "./public/build")

	r.GET("/hello", func(c *gin.Context) {
		html(c, func() error { return views.Hello("dzovi", now()).Render(c.Request.Context(), c.Writer) })
	})

	r.GET("/greeting", func(c *gin.Context) {
		html(c, func() error { return views.Greeting("dzovi", now()).Render(c.Request.Context(), c.Writer) })
	})

	r.POST("/greet", func(c *gin.Context) {
		name := strings.TrimSpace(c.PostForm("name"))
		if name == "" {
			name = "stranger"
		}
		html(c, func() error { return views.Greeting(name, now()).Render(c.Request.Context(), c.Writer) })
	})

	// Phase 6: persistence. GET renders the full page, POST inserts and returns
	// just the refreshed list fragment for HTMX to swap in.
	r.GET("/users", func(c *gin.Context) {
		users, err := db.ListUsers(c.Request.Context())
		if err != nil {
			c.String(500, "list users: %v", err)
			return
		}
		html(c, func() error { return views.UsersPage(users).Render(c.Request.Context(), c.Writer) })
	})

	r.POST("/users", func(c *gin.Context) {
		name := strings.TrimSpace(c.PostForm("name"))
		if name == "" {
			c.String(400, "name is required")
			return
		}
		if _, err := db.CreateUser(c.Request.Context(), name); err != nil {
			c.String(500, "create user: %v", err)
			return
		}
		users, err := db.ListUsers(c.Request.Context())
		if err != nil {
			c.String(500, "list users: %v", err)
			return
		}
		html(c, func() error { return views.UserList(users).Render(c.Request.Context(), c.Writer) })
	})

	r.Run(":8080")
}
