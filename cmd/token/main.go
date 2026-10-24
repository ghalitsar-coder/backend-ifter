// Command token mints a development access token with the configured signing
// key. It is a local debugging aid, not part of the deployed service.
package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
)

func main() {
	subject := flag.String("sub", "dev-user", "token subject")
	rolesFlag := flag.String("roles", "admin", "comma-separated roles")
	schoolID := flag.Int64("school", 1, "school id")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	roles := make([]string, 0, 2)
	for _, role := range strings.Split(*rolesFlag, ",") {
		if role = strings.TrimSpace(role); role != "" {
			roles = append(roles, role)
		}
	}

	token, err := auth.NewTokenService(cfg.Auth).Issue(*subject, roles, *schoolID)
	if err != nil {
		log.Fatalf("issue token: %v", err)
	}
	fmt.Println(token)
}
