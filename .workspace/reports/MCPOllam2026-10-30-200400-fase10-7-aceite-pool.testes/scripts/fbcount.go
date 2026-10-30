// fbcount: escuta 127.0.0.1:11598 e conta cada pedido (prova de que o fallback foi ou nao chamado).
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	http.ListenAndServe("127.0.0.1:11598", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(os.Stdout, "%s HIT %s %s\n", time.Now().Format("15:04:05.000"), r.Method, r.URL.Path)
		os.Stdout.Sync()
		http.Error(w, `{"error":"fbcount"}`, http.StatusInternalServerError)
	}))
}
