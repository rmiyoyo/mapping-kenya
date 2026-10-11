package site

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

func num(n int) string {
	if n < 0 {
		return "-" + num(-n)
	}
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func decimal(f float64) string {
	s := strconv.FormatFloat(f, 'f', 1, 64)
	whole, frac, _ := strings.Cut(s, ".")
	n, _ := strconv.Atoi(whole)
	return num(n) + "." + frac
}

func round(f float64) int {
	return int(math.Round(f))
}

func coord(f float64) string {
	return strconv.FormatFloat(f, 'f', 4, 64)
}

func km(f float64) string {
	if f < 10 {
		return strconv.FormatFloat(f, 'f', 1, 64) + " km"
	}
	return num(round(f)) + " km"
}

func titleCase(s string) string {
	upper := true
	return strings.Map(func(r rune) rune {
		out := unicode.ToLower(r)
		if upper {
			out = unicode.ToUpper(r)
		}
		upper = r == ' ' || r == '-' || r == '/' || r == '('
		return out
	}, s)
}

func countyURL(code int) string {
	return "/counties/" + strconv.Itoa(code)
}

func constituencyURL(name string) string {
	return "/constituencies/" + url.PathEscape(name)
}

func wardURL(code string) string {
	return "/wards/" + url.PathEscape(code)
}

func postcodeURL(code string) string {
	return "/postcodes/" + url.PathEscape(code)
}

func atURL(lat, lng float64) string {
	return fmt.Sprintf("/at?lat=%s&lng=%s", coord(lat), coord(lng))
}
