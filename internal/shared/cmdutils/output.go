package cmdutils

import "fmt"

const logo = "🐬"

func PrintResponse(text string) {
	if text == "" {
		return
	}

	fmt.Printf("\n%s crystaldolphin\n%s\n\n", logo, text)
}
