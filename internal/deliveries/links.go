package deliveries

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ---------------------------------------------------------------------------
// Listas
// ---------------------------------------------------------------------------

var shorteners = []string{
	"bit.ly", "tinyurl.com", "cutt.ly", "is.gd", "t.co", "goo.gl", "ow.ly", "rb.gy", "shorturl.at",
}

var repositoryHosts = []string{"github.com", "gitlab.com", "bitbucket.org"}

var videoHosts = []string{
	"youtube.com", "youtu.be", "vimeo.com", "drive.google.com", "loom.com", "onedrive.live.com",
}

var adultDomains = []string{
	"pornhub.com", "xvideos.com", "xnxx.com", "xhamster.com", "redtube.com", "youporn.com",
	"spankbang.com", "onlyfans.com", "fansly.com", "brazzers.com", "chaturbate.com", "stripchat.com",
	"bongacams.com", "livejasmin.com", "cam4.com", "rule34.xxx", "e-hentai.org", "nhentai.net",
	"eporner.com", "tnaflix.com", "camsoda.com",
}

var adultTerms = []string{
	"porn", "xxx", "hentai", "nsfw", "onlyfans", "xvideos", "xnxx", "xhamster", "camgirl", "erotic", "fetish",
}

// ---------------------------------------------------------------------------
// Links
// ---------------------------------------------------------------------------

// Verificação
func hostIn(host string, list []string) bool {
	for _, d := range list {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// Verificação
func blockedHost(host string) bool {
	if hostIn(host, adultDomains) {
		return true
	}
	for _, t := range adultTerms {
		if strings.Contains(host, t) {
			return true
		}
	}
	return false
}

// Verificação
func publicHost(host string) bool {
	if host == "" || host == "localhost" || net.ParseIP(host) != nil {
		return false
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if l == "" || len(l) > 63 || strings.HasPrefix(l, "-") || strings.HasSuffix(l, "-") {
			return false
		}
		for _, r := range l {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	tld := labels[len(labels)-1]
	if strings.HasPrefix(tld, "xn--") {
		return true
	}
	if len(tld) < 2 {
		return false
	}
	for _, r := range tld {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// Validação
func checkLink(f Field, raw string) string {
	invalid := fmt.Sprintf("O campo %q precisa de um link válido, com endereço público iniciado por http:// ou https://.", f.Label)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return invalid
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if !publicHost(host) {
		return invalid
	}
	if hostIn(host, shorteners) {
		return msgShortener
	}
	if blockedHost(host) {
		return msgBlockedDomain
	}
	switch f.Format {
	case FormatRepository:
		return checkRepository(f, host, u)
	case FormatVideo:
		return checkVideo(f, host, u)
	}
	return ""
}

// Validação
func checkRepository(f Field, host string, u *url.URL) string {
	if !hostIn(host, repositoryHosts) {
		return fmt.Sprintf("O campo %q aceita apenas links do GitHub, GitLab ou Bitbucket.", f.Label)
	}
	segments := 0
	for _, s := range strings.Split(u.Path, "/") {
		if s != "" {
			segments++
		}
	}
	if segments < 2 {
		return fmt.Sprintf("O campo %q precisa do link do repositório, no formato usuario/repositorio.", f.Label)
	}
	return ""
}

// Validação
func checkVideo(f Field, host string, u *url.URL) string {
	if !hostIn(host, videoHosts) {
		return fmt.Sprintf("O campo %q aceita apenas links do YouTube, Vimeo, Google Drive, Loom ou OneDrive.", f.Label)
	}
	incomplete := fmt.Sprintf("O campo %q precisa do link completo do vídeo.", f.Label)
	path := strings.Trim(u.Path, "/")
	switch {
	case hostIn(host, []string{"youtube.com"}):
		ok := (path == "watch" && u.Query().Get("v") != "") || (path == "playlist" && u.Query().Get("list") != "") ||
			strings.HasPrefix(path, "shorts/") || strings.HasPrefix(path, "embed/") || strings.HasPrefix(path, "live/")
		if !ok {
			return incomplete
		}
	case path == "":
		return incomplete
	}
	return ""
}

const (
	msgShortener     = "Links encurtados não são aceitos. Informe o endereço completo."
	msgBlockedDomain = "Domínio não permitido para entregas acadêmicas."
)
