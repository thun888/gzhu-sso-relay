package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/thun888/gzhu-sso-relay/internal/config"
	"github.com/thun888/gzhu-sso-relay/internal/cryptox"
	"github.com/thun888/gzhu-sso-relay/internal/store"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

func main() {
	if len(os.Args) < 3 {
		usage()
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		die(err)
	}
	db, err := store.Open(cfg.Database.DSN)
	if err != nil {
		die(err)
	}
	if err := store.Migrate(db); err != nil {
		die(err)
	}
	if err := store.Seed(db); err != nil {
		die(err)
	}

	ctx := context.Background()
	switch os.Args[1] + " " + os.Args[2] {
	case "client create":
		createClient(ctx, db, os.Args[3:])
	case "client list":
		listClients(ctx, db)
	case "client delete":
		deleteClient(ctx, db, os.Args[3:])
	default:
		usage()
		os.Exit(2)
	}
}

func createClient(ctx context.Context, db *gorm.DB, args []string) {
	fs := flag.NewFlagSet("client create", flag.ExitOnError)
	id := fs.String("id", "", "client_id")
	name := fs.String("name", "", "应用名称")
	redirects := fs.String("redirect", "", "允许的回调地址，逗号分隔")
	scopes := fs.String("scopes", "", "额外允许的 scope，逗号分隔")
	public := fs.Bool("public", false, "公开客户端，不校验密钥")
	_ = fs.Parse(args)

	if *id == "" || *name == "" || *redirects == "" {
		die(fmt.Errorf("-id、-name、-redirect 均为必填"))
	}

	var secret, hash string
	if !*public {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			die(err)
		}
		secret = base64.RawURLEncoding.EncodeToString(buf)
		h, err := cryptox.HashPassword(secret)
		if err != nil {
			die(err)
		}
		hash = h
	}

	client := model.OAuthClient{
		ClientID:     *id,
		SecretHash:   hash,
		Name:         *name,
		RedirectURLs: splitList(*redirects),
		IsPublic:     *public,
	}
	if err := db.WithContext(ctx).Create(&client).Error; err != nil {
		die(fmt.Errorf("创建应用失败: %w", err))
	}

	for _, scope := range splitList(*scopes) {
		link := model.ClientAllowedScope{ClientID: *id, ScopeName: scope}
		if err := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error; err != nil {
			die(fmt.Errorf("写入 %s 的 scope 失败: %w", scope, err))
		}
	}

	fmt.Printf("client_id      %s\n", client.ClientID)
	if secret != "" {
		fmt.Printf("client_secret  %s\n", secret)
	}
	fmt.Printf("redirect_urls  %s\n", strings.Join(client.RedirectURLs, ", "))
	fmt.Printf("public         %t\n", client.IsPublic)
	if secret != "" {
		fmt.Println("\nclient_secret 只显示一次，服务端仅保存其哈希。")
	}
}

func listClients(ctx context.Context, db *gorm.DB) {
	var clients []model.OAuthClient
	if err := db.WithContext(ctx).Order("id").Find(&clients).Error; err != nil {
		die(err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CLIENT_ID\tNAME\tPUBLIC\tREDIRECT_URLS")
	for _, c := range clients {
		fmt.Fprintf(w, "%s\t%s\t%t\t%s\n", c.ClientID, c.Name, c.IsPublic, strings.Join(c.RedirectURLs, ","))
	}
	_ = w.Flush()
}

func deleteClient(ctx context.Context, db *gorm.DB, args []string) {
	fs := flag.NewFlagSet("client delete", flag.ExitOnError)
	id := fs.String("id", "", "client_id")
	_ = fs.Parse(args)

	if *id == "" {
		die(fmt.Errorf("-id 为必填"))
	}

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("client_id = ?", *id).Delete(&model.ClientAllowedScope{}).Error; err != nil {
			return err
		}
		if err := tx.Where("client_id = ?", *id).Delete(&model.GroupOAuthClient{}).Error; err != nil {
			return err
		}
		return tx.Where("client_id = ?", *id).Delete(&model.OAuthClient{}).Error
	})
	if err != nil {
		die(fmt.Errorf("删除应用失败: %w", err))
	}
	fmt.Printf("已删除 %s\n", *id)
}

func splitList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func usage() {
	fmt.Fprintln(os.Stderr, `relayctl 管理命令

  relayctl client create -id <client_id> -name <名称> -redirect <回调地址[,地址]> [-scopes a,b] [-public]
  relayctl client list
  relayctl client delete -id <client_id>

需要与 relay 相同的环境变量（RELAY_DB_DSN 等）。`)
}

func die(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
