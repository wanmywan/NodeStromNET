package main

import "C"

// Exported function for DLL entry point
//export RunAgent
func RunAgent() {
	IsDLL = true
	main()
}
