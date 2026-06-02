package deploy

import "fmt"

// Event 是部署過程回報的進度。TUI 之後會把它接到進度條；headless 時就印出來。
type Event struct {
	Phase string  // tag / build / apply / setimage / rollout / fail / rollback / done
	Msg   string  // 人看的訊息或一行 build log
	Pct   float64 // rollout 階段 0..1；其他階段 -1
}

// Emitter 收事件。
type Emitter func(Event)

// BuildTag 產生官方風 tag：v0.1.0_YYYYMMDD.N
func BuildTag(versionBase, date string, n int) string {
	return fmt.Sprintf("%s_%s.%d", versionBase, date, n)
}
