package model

import "strings"

// Service 是一個 WEDA deployment 在列表上的精簡視圖。
type Service struct {
	Name     string
	Ready    string // "1/1"
	UpToDate int
	Age      string // "34d" / "2h" / "15m"
	Image    string
}

// ShortImage 去掉 registry/project 前綴，只留 repo:tag，列表才不會被長字串塞爆。
// e.g. registry.example.com/project/my-svc:v1.0.0 -> my-svc:v1.0.0
func (s Service) ShortImage() string {
	if i := strings.LastIndex(s.Image, "/"); i >= 0 {
		return s.Image[i+1:]
	}
	return s.Image
}
