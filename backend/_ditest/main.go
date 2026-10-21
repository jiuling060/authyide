package main

import (
	"fmt"

	appv1 "github.com/chaitin/MonkeyCode/backend/biz/app/handler/v1"
	subv1 "github.com/chaitin/MonkeyCode/backend/biz/subscription/handler/v1"
)

func main() {
	var a *appv1.AppHandler
	var b *subv1.Handler
	fmt.Println("APP        =", fmt.Sprintf("%T", a))
	fmt.Println("SUBSCRIPTION =", fmt.Sprintf("%T", b))
}
