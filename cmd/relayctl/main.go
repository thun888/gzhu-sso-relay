package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/thun888/gzhu-sso-relay/internal/config"
	"github.com/thun888/gzhu-sso-relay/internal/cryptox"
	"github.com/thun888/gzhu-sso-relay/internal/logo"
	"github.com/thun888/gzhu-sso-relay/internal/store"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

func main() {
	args, configPath := splitConfigFlag(os.Args[1:])
	if len(args) < 2 {
		usage()
		os.Exit(2)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		die(err)
	}
	db, err := store.Open(cfg.Database.DSN)
	if err != nil {
		die(err)
	}
	if err := store.Migrate(db, logo.New(cfg.Server.LogoDir)); err != nil {
		die(err)
	}
	if err := store.Seed(db); err != nil {
		die(err)
	}

	ctx := context.Background()
	switch args[0] + " " + args[1] {
	case "client create":
		createClient(ctx, db, args[2:])
	case "client list":
		listClients(ctx, db)
	case "client delete":
		deleteClient(ctx, db, args[2:])
	case "user promote":
		promoteUser(ctx, db, args[2:])
	case "user import":
		importUsers(ctx, db, cfg, args[2:])
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

	if *name == "" || *redirects == "" {
		die(fmt.Errorf("-name、-redirect 均为必填"))
	}

	clientID := *id
	if clientID == "" {
		generated, err := cryptox.NewClientID()
		if err != nil {
			die(err)
		}
		clientID = generated
	}

	var secret, hash string
	if !*public {
		plain, hashed, err := cryptox.NewSecret()
		if err != nil {
			die(err)
		}
		secret, hash = plain, hashed
	}

	client := model.OAuthClient{
		ClientID:     clientID,
		SecretHash:   hash,
		Name:         *name,
		RedirectURLs: splitList(*redirects),
		IsPublic:     *public,
	}
	if err := db.WithContext(ctx).Create(&client).Error; err != nil {
		die(fmt.Errorf("创建应用失败: %w", err))
	}

	for _, scope := range splitList(*scopes) {
		link := model.ClientAllowedScope{ClientID: clientID, ScopeName: scope}
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

func promoteUser(ctx context.Context, db *gorm.DB, args []string) {
	fs := flag.NewFlagSet("user promote", flag.ExitOnError)
	id := fs.String("id", "", "学号")
	revoke := fs.Bool("revoke", false, "取消管理员身份")
	_ = fs.Parse(args)

	if *id == "" {
		die(fmt.Errorf("-id 为必填"))
	}

	kind := int16(model.UserTypeAdmin)
	if *revoke {
		kind = model.UserTypeNormal
	}

	result := db.WithContext(ctx).Model(&model.User{}).Where("id = ?", *id).Update("type", kind)
	if result.Error != nil {
		die(fmt.Errorf("更新用户失败: %w", result.Error))
	}
	if result.RowsAffected == 0 {
		die(fmt.Errorf("用户 %s 不存在，需先登录一次 relay", *id))
	}

	if *revoke {
		fmt.Printf("%s 已取消管理员\n", *id)
		return
	}
	fmt.Printf("%s 已设为管理员\n", *id)
}

// 本地校验必然失败（不是合法 PHC 格式），预导入用户首次登录会走上游认证并覆盖此值。
const placeholderHash = "!preimport"

func importUsers(ctx context.Context, db *gorm.DB, cfg *config.Config, args []string) {
	fs := flag.NewFlagSet("user import", flag.ExitOnError)
	groupCode := fs.String("group", "", "导入后加入的用户组 code")
	// flag 包遇到首个位置参数就停止解析，先重排保证 flag 写在文件名后面也能生效
	_ = fs.Parse(flagsFirst(fs, args))
	if fs.NArg() < 1 {
		die(fmt.Errorf("需要提供用户列表文件路径，- 表示标准输入"))
	}

	type entry struct {
		id   string
		name string
	}
	var entries []entry
	seen := make(map[string]struct{})

	scanner := bufio.NewScanner(os.Stdin)
	if path := fs.Arg(0); path != "-" {
		f, err := os.Open(path)
		if err != nil {
			die(fmt.Errorf("打开文件失败: %w", err))
		}
		defer f.Close()
		scanner = bufio.NewScanner(f)
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		id, name := line, ""
		if i := strings.IndexByte(line, ','); i >= 0 {
			id, name = strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:])
		}
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		entries = append(entries, entry{id: id, name: name})
	}
	if err := scanner.Err(); err != nil {
		die(fmt.Errorf("读取用户列表失败: %w", err))
	}
	if len(entries) == 0 {
		die(fmt.Errorf("用户列表为空"))
	}

	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.id)
	}
	var found []string
	if err := db.WithContext(ctx).Model(&model.User{}).Where("id IN ?", ids).Pluck("id", &found).Error; err != nil {
		die(fmt.Errorf("查询已有用户失败: %w", err))
	}
	existing := make(map[string]struct{}, len(found))
	for _, id := range found {
		existing[id] = struct{}{}
	}

	var users []model.User
	var noDomain []string
	for _, e := range entries {
		if _, ok := existing[e.id]; ok {
			continue
		}
		suffix, ok := cfg.Server.EmailDomains.Suffix(e.id)
		if !ok {
			noDomain = append(noDomain, e.id)
			continue
		}
		name := e.name
		if name == "" {
			name = e.id
		}
		users = append(users, model.User{
			ID:           e.id,
			Name:         name,
			PasswordHash: placeholderHash,
			Email:        e.id + "@" + suffix,
			Type:         model.UserTypeNormal,
			Status:       model.UserStatusActive,
		})
	}
	if len(users) > 0 {
		if err := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&users).Error; err != nil {
			die(fmt.Errorf("写入用户失败: %w", err))
		}
	}

	joined := 0
	if *groupCode != "" {
		var g model.Group
		err := db.WithContext(ctx).Where("code = ?", *groupCode).First(&g).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			die(fmt.Errorf("用户组 %s 不存在", *groupCode))
		}
		if err != nil {
			die(fmt.Errorf("查询用户组失败: %w", err))
		}
		if g.IsUpstream {
			die(fmt.Errorf("用户组 %s 由上游同步生成，不能手工维护成员", g.Code))
		}
		links := make([]model.UserGroup, 0, len(entries))
		for _, e := range entries {
			links = append(links, model.UserGroup{UserID: e.id, GroupID: g.ID})
		}
		if err := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&links).Error; err != nil {
			die(fmt.Errorf("写入用户组成员失败: %w", err))
		}
		joined = len(links)
		fmt.Printf("已把 %d 个用户加入组 %s\n", joined, g.Code)
	}

	fmt.Printf("共 %d 条：新建 %d，已存在 %d，无邮箱后缀跳过 %d\n",
		len(entries), len(users), len(entries)-len(users)-len(noDomain), len(noDomain))
	if len(noDomain) > 0 {
		fmt.Printf("无邮箱后缀: %s\n", strings.Join(noDomain, ", "))
	}
}

// flagsFirst 把 flag 挪到位置参数之前，未知 flag 原样保留交给 Parse 报错。
func flagsFirst(fs *flag.FlagSet, args []string) []string {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			rest = append(rest, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		takesValue := false
		if j := strings.IndexByte(name, '='); j >= 0 {
			name = name[:j]
		} else if f := fs.Lookup(name); f != nil {
			_, isBool := f.Value.(interface{ IsBoolFlag() bool })
			takesValue = !isBool && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-")
		}
		flags = append(flags, arg)
		if takesValue {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, rest...)
}

// splitConfigFlag 把 -config 从参数里摘出来，其余参数保持原有顺序。
func splitConfigFlag(args []string) ([]string, string) {
	path := "config.yaml"
	rest := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-config" || arg == "--config":
			if i+1 < len(args) {
				path = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "-config="):
			path = strings.TrimPrefix(arg, "-config=")
		case strings.HasPrefix(arg, "--config="):
			path = strings.TrimPrefix(arg, "--config=")
		default:
			rest = append(rest, arg)
		}
	}
	return rest, path
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

  relayctl [-config 路径] <命令>

  relayctl client create -name <名称> -redirect <回调地址[,地址]> [-id 自定义标识] [-scopes a,b] [-public]
  relayctl client list
  relayctl client delete -id <client_id>
  relayctl user promote -id <学号> [-revoke]
  relayctl user import <文件|-> [-group <组code>]

    每行一个学号，可带姓名：20240001,张三。# 开头的行跳过。
    预导入的用户本地密码不可用，首次登录走上游认证并补全资料。

-config 缺省为当前目录的 config.yaml，只需其中 database.dsn 可用。`)
}

func die(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
