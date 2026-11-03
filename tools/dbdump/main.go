// Command dbdump prints the FluxDM download list (diagnostics).
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"fluxdm/internal/store"
)

func main() {
	base, _ := os.UserConfigDir()
	st, err := store.Open(filepath.Join(base, "FluxDM", "fluxdm.db"))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer st.Close()
	items, _ := st.ListItems()
	for _, it := range items {
		fmt.Printf("%s %-10s %-11s %d/%d %s\n   path=%s dir=%s\n   err=%s\n", it.ID, it.Kind, it.Status, it.Downloaded, it.Size, it.URL, it.Path, it.Dir, it.Error)
	}
}
