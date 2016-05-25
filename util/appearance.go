package util

import "fmt"

func Bold(text string) string {
	return fmt.Sprintf("\033[1m%s\033[0m", text)
}

func StringifyMemory(value uint64) string {
	if value == 0 {
		return "-"
	}
	d := "b"
	f := "%.1f%s"
	v := float64(value)
	if value >= 1024 && value < 1024*1024 {
		d = "kb"
		v /= 1024
	} else if value >= 1024*1024 {
		d = "mb"
		v /= 1024 * 1024
	} else {
		f = "%.0f%s"
	}
	return fmt.Sprintf(f, v, d)
}

func StringifyLoad(value float64) string {
	if value > 1 {
		value = 1
	}
	return fmt.Sprintf("%.0f%%", value*100)
}
