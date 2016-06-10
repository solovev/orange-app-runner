package util

import "fmt"

// Bold возвращает введенную строку с кодом выделения в терминале.
// Пример:
//	text: "hello world"
//	return:	"\033[1mhello world\033[0m"
func Bold(text string) string {
	return fmt.Sprintf("\033[1m%s\033[0m", text)
}

// StringifyMemory принимает в параметр <value> количество байт и
// возращает удобно читаемую (отформатированную) строку с суффиксом размерности.
// Пример:
//	value: "5000"
//	return:	"4.8kb"
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

// StringifyLoad переводит значение <value> из диапазона [0.0, 1.0] в %'ый вид.
// Пример:
//	value: "0.34"
//	return:	"34%"
func StringifyLoad(value float64) string {
	if value > 1 {
		value = 1
	}
	if value < 0 {
		value = 0
	}
	return fmt.Sprintf("%.0f%%", value*100)
}
