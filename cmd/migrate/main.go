package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"invest_intraday/internal/a_submodule/migrate"
	"invest_intraday/internal/a_technical/config"
)

func main() {
	migrationsDir := flag.String("dir", "migrations", "путь до каталога с миграциями")
	forceVersion := flag.Int("force", -1, "принудительно установить версию миграции")
	configPathFlag := flag.String("config", "", "путь до конфигурационного файла")
	flag.Parse()

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.Fatalf("не удалось загрузить часовую зону Europe/Moscow: %v", err)
	}
	time.Local = loc

	databaseURL := resolveDatabaseURL(*configPathFlag)
	if strings.TrimSpace(databaseURL) == "" {
		log.Fatal("не удалось получить DATABASE_URL из окружения или конфигурации")
	}

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		log.Fatalf("не удалось открыть соединение с PostgreSQL: %v", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("ошибка при закрытии соединения с БД: %v", cerr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("не удалось выполнить ping PostgreSQL: %v", err)
	}

	if *forceVersion >= 0 {
		if err := migrate.Force(db, *migrationsDir, *forceVersion); err != nil {
			log.Fatalf("ошибка при выполнении force миграции: %v", err)
		}
		log.Printf("успешно выполнен force до версии %d", *forceVersion)
		return
	}

	if err := migrate.Up(db, *migrationsDir); err != nil {
		log.Fatalf("ошибка при применении миграций: %v", err)
	}

	log.Println("миграции успешно применены")
}

func resolveDatabaseURL(configPathFlag string) string {
	if envURL := os.Getenv("DATABASE_URL"); strings.TrimSpace(envURL) != "" {
		return envURL
	}

	cfgPath := configPathFlag
	if strings.TrimSpace(cfgPath) == "" {
		cfgPath = os.Getenv("CONFIG_PATH")
	}
	if strings.TrimSpace(cfgPath) == "" {
		cfgPath = "config.json"
	}

	cfg, err := config.FromFile(cfgPath)
	if err != nil {
		log.Fatalf("не удалось загрузить конфигурацию %q: %v", cfgPath, err)
	}

	return cfg.DatabaseURL
}
