// health-trail: a personal health record that lives on your own computer,
// with a Mac window, a web UI a phone can open, and MCP tools for Claude.
//
//	health-trail             the desktop app (window + local server)
//	health-trail serve       the server only (no window), e.g. on a mini PC
//	health-trail mcp         MCP over stdio, for Claude Desktop / Claude Code
//	health-trail info        where the data is, how to connect
//
// The app itself is internal/desktop; this only reads the command line.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/lee/health-trail/internal/desktop"
)

// version is set at build time (-X main.version=…)
var version = "0.1.0"

func main() {
	desktop.Version = version
	cmd, args := "app", os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	// macOS passes -psn_… to apps opened from Finder on older systems
	if len(args) > 0 && strings.HasPrefix(args[0], "-psn_") {
		args = nil
	}
	fl := flag.NewFlagSet(cmd, flag.ExitOnError)
	dataDir := fl.String("data", desktop.DefaultDataDir(), "数据目录（也可用环境变量 HEALTH_TRAIL_DATA）")
	port := fl.String("port", desktop.DefaultPort, "本地服务端口")
	lan := fl.Bool("lan", false, "serve：允许局域网/Tailscale 中的设备（手机）访问，需令牌")
	fl.Parse(args)

	switch cmd {
	case "mcp":
		desktop.RunMCP(*dataDir)
	case "serve":
		desktop.RunServe(*dataDir, *port, *lan)
	case "app":
		desktop.RunApp(*dataDir, *port)
	case "info":
		desktop.RunInfo(*dataDir, *port)
	case "version":
		fmt.Println(version)
	default:
		fmt.Fprintf(os.Stderr, "用法：health-trail [app|serve|mcp|info|version] [-data 目录] [-port 端口] [-lan]\n")
		os.Exit(2)
	}
}
