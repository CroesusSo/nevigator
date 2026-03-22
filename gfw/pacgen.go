package gfw

import (
	"fmt"
	"log"
	"os"
)

// RegeneratePAC reads user and GFW rules, generates PAC, and writes it atomically.
func RegeneratePAC(userPath, gfwPath, pacPath string, defaultExternalProxy string, enableURLGlob bool, enabled bool) error {
	userRules, err := LoadRules(userPath)
	if err != nil {
		log.Printf("[PAC] Failed to load user rules: %v", err)
		userRules = []Rule{}
	}

	var gfwRules []Rule
	if enabled {
		gfwRules, err = LoadRules(gfwPath)
		if err != nil {
			log.Printf("[PAC] Failed to load GFW rules: %v", err)
			gfwRules = []Rule{}
		}
	} else {
		gfwRules = []Rule{}
	}

	meta := FWMeta{
		Source:               "gfwlist",
		RuleCount:            len(gfwRules),
		DefaultExternalProxy: defaultExternalProxy,
		EnableURLGlob:        enableURLGlob,
	}
	content := GeneratePAC(userRules, gfwRules, meta)

	tmpPath := pacPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write PAC tmp file: %v", err)
	}
	if err := os.Rename(tmpPath, pacPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename PAC file: %v", err)
	}
	log.Printf("[PAC] Regenerated: %d user + %d GFW rules", len(userRules), len(gfwRules))
	return nil
}
