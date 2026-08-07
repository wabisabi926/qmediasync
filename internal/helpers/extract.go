package helpers

import (
	"regexp"
	"strconv"
)

// 提取季编号
func ExtractSeasonsFromSeasonPath(text string) int {
	if len(text) == 0 {
		return -1
	}
	f := string(text[0])
	if f != "s" && f != "S" {
		AppLogger.Errorf("提取季编号失败，路径 %s 不是以s或S开头", text)
		return -1
	}
	// 如果text的首字母是s，则转成大写的S
	if f == "s" {
		text = "S" + text[1:]
	}
	pattern := `(?i)season\s+0*(\d+)`
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(text)
	if matches != nil {
		// 将字符串数字转换为整数
		num, err := strconv.Atoi(matches[1])
		if err == nil {
			return num
		} else {
			AppLogger.Errorf("提取季编号失败，路径 %s 匹配的季编号 %s 不是整数", text, matches[1])
		}
	} else {
		AppLogger.Errorf("提取季编号失败，路径 %s 没有匹配的季编号", text)
	}
	pattern = `(?i)s(\d+)$`
	re = regexp.MustCompile(pattern)
	matches = re.FindStringSubmatch(text)
	if matches != nil {
		// 将字符串数字转换为整数
		num, err := strconv.Atoi(matches[1])
		if err == nil {
			return num
		} else {
			AppLogger.Errorf("提取季编号失败，路径 %s 匹配的季编号 %s 不是整数", text, matches[1])
		}
	} else {
		AppLogger.Errorf("提取季编号失败，路径 %s 没有匹配的季编号", text)
	}
	return -1
}
