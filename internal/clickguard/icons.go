package clickguard

import (
	"encoding/base64"
	"fmt"
	rand2 "math/rand/v2"
	"strings"
)

const (
	imageWidth  = 300
	imageHeight = 180
)

type region struct {
	x1 int
	y1 int
	x2 int
	y2 int
}

var regions = []region{
	{x1: 43, y1: 52, x2: 105, y2: 94},
	{x1: 195, y1: 52, x2: 257, y2: 94},
	{x1: 116, y1: 110, x2: 184, y2: 154},
}

func placeIcons(icons []string) []point {
	regionOrder := []int{0, 1, 2}
	rand2.Shuffle(len(regionOrder), func(i, j int) {
		regionOrder[i], regionOrder[j] = regionOrder[j], regionOrder[i]
	})
	points := make([]point, len(icons))
	for i, icon := range icons {
		area := regions[regionOrder[i]]
		points[i] = point{
			Icon: icon,
			X:    area.x1 + rand2.IntN(area.x2-area.x1+1),
			Y:    area.y1 + rand2.IntN(area.y2-area.y1+1),
		}
	}
	return points
}

func renderChallenge(points []point) (string, error) {
	var challenge strings.Builder
	fmt.Fprintf(&challenge, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		imageWidth, imageHeight, imageWidth, imageHeight)
	challenge.WriteString(`<rect width="300" height="180" rx="2" fill="#f1efe6"/>`)
	for range 90 {
		x := rand2.IntN(imageWidth)
		y := rand2.IntN(imageHeight)
		opacity := 0.12 + rand2.Float64()*0.2
		fmt.Fprintf(&challenge, `<circle cx="%d" cy="%d" r="1" fill="#6c6b63" opacity="%.2f"/>`, x, y, opacity)
	}
	for range 4 {
		x1 := rand2.IntN(imageWidth)
		y1 := rand2.IntN(imageHeight)
		x2 := rand2.IntN(imageWidth)
		y2 := rand2.IntN(imageHeight)
		fmt.Fprintf(&challenge, `<path d="M%d %d L%d %d" stroke="#a7a59b" stroke-width="1" opacity=".65"/>`, x1, y1, x2, y2)
	}
	for _, item := range points {
		challenge.WriteString(iconElement(item.Icon, item.X, item.Y, 38, "#27483f"))
	}
	challenge.WriteString(`</svg>`)
	return svgDataURL(challenge.String()), nil
}

func renderIconPreview(icon string) string {
	var preview strings.Builder
	preview.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="36" height="36" viewBox="0 0 36 36">`)
	preview.WriteString(`<rect width="36" height="36" rx="8" fill="#f1efe6"/>`)
	preview.WriteString(iconElement(icon, 18, 18, 24, "#27483f"))
	preview.WriteString(`</svg>`)
	return svgDataURL(preview.String())
}

func svgDataURL(value string) string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(value))
}

func iconElement(icon string, centerX, centerY, size int, fill string) string {
	scale := float64(size) / 32
	left := float64(centerX) - float64(size)/2
	top := float64(centerY) - float64(size)/2
	open := fmt.Sprintf(`<g transform="translate(%.2f %.2f) scale(%.4f)" fill="%s">`, left, top, scale, fill)
	return open + iconBody(icon, fill) + `</g>`
}

func iconBody(icon, fill string) string {
	switch icon {
	case "sun":
		return `<circle cx="16" cy="16" r="6"/><path stroke="` + fill + `" stroke-width="2.4" stroke-linecap="round" d="M16 2v5M16 25v5M2 16h5M25 16h5M6.1 6.1l3.5 3.5M22.4 22.4l3.5 3.5M25.9 6.1l-3.5 3.5M9.6 22.4l-3.5 3.5"/>`
	case "heart":
		return `<path d="M16 29C13 26 4 20 4 12C4 6 12 2 16 8C20 2 28 6 28 12C28 20 19 26 16 29Z"/>`
	case "star":
		return `<path d="M16 2l3.2 9.9h10.4l-8.4 6.1 3.2 9.9-8.4-6.1-8.4 6.1 3.2-9.9-8.4-6.1h10.4Z"/>`
	case "cloud":
		return `<path d="M7 26c-3.5 0-5-2.3-5-5 0-3.2 2.2-4.8 4.9-4.8.4-3.6 3.1-6.2 7.4-6.2 3.1 0 5.4 1.4 6.7 3.6.5-.2 1.2-.3 1.8-.3 3.5 0 6.2 2.6 6.2 6.1 3 0 5 2.2 5 4.8 0 2.8-1.8 4.8-5 4.8H7Z"/>`
	case "leaf":
		return `<path d="M27 4C12 4 4 12 5 27c15 1 23-7 22-23Z"/><path stroke="#f1efe6" stroke-width="1.6" stroke-linecap="round" d="M8 24c4-6 9-11 16-16"/>`
	case "droplet":
		return `<path d="M16 3C16 3 5 16 5 21c0 6 5 9 11 9s11-3 11-9C27 16 16 3 16 3Z"/>`
	case "flower":
		return `<circle cx="16" cy="7" r="5"/><circle cx="24" cy="13" r="5"/><circle cx="21" cy="23" r="5"/><circle cx="11" cy="23" r="5"/><circle cx="8" cy="13" r="5"/><circle cx="16" cy="16" r="4" fill="#f1efe6"/>`
	case "diamond":
		return `<path d="M16 2l14 14-14 14L2 16Z"/>`
	default:
		return `<circle cx="16" cy="16" r="11"/>`
	}
}
