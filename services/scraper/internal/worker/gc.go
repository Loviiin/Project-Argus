package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const orphanProfileTTL = 90 * time.Minute

// StartProfileSweeper periodicamente verifica e remove pastas de perfis de browsers
// temporários (órfãos) que ficaram para trás após um crash do worker ou vazamento.
func StartProfileSweeper() {
	fmt.Println("[GC] Iniciando Profile Sweeper...")
	for {
		time.Sleep(15 * time.Minute)
		sweepOrphanProfiles(os.TempDir(), orphanProfileTTL)
	}
}

// sweepOrphanProfiles contém a lógica principal isolada para facilitar testes unitários
func sweepOrphanProfiles(baseDir string, ttl time.Duration) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		fmt.Printf("[GC] Erro lendo diretório base %s: %v\n", baseDir, err)
		return
	}

	removedCount := 0
	now := time.Now()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, "argus_profile_") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if now.Sub(info.ModTime()) > ttl {
			fullPath := filepath.Join(baseDir, name)
			if err := os.RemoveAll(fullPath); err != nil {
				fmt.Printf("[GC] Erro removendo perfil órfão %s: %v\n", fullPath, err)
			} else {
				removedCount++
			}
		}
	}

	if removedCount > 0 {
		fmt.Printf("[GC] 🧹 Sweeper removeu %d perfis órfãos do diretório temporário.\n", removedCount)
	}
}
