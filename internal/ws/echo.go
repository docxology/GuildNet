package ws

import (
	"context"
	"net/http"
	"time"

	"nhooyr.io/websocket"
)

func EchoHandler(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
	if err != nil { return }
	defer c.Close(websocket.StatusNormalClosure, "bye")

	c.SetReadLimit(1 << 20) // 1MB
	deadline := 10 * time.Second

	for {
		ctx, cancel := context.WithTimeout(r.Context(), deadline)
		typ, data, err := c.Read(ctx)
		cancel()
		if err != nil { return }

		wctx, wcancel := context.WithTimeout(r.Context(), deadline)
		err = c.Write(wctx, typ, data)
		wcancel()
		if err != nil { return }
	}
}
