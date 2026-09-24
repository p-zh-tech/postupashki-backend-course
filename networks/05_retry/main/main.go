package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"
)

var Usage string = "Usage: run.sh <адрес ресурса> [--method <метод>] [--max-attempts <число>] [--idempotency-key <значение>]"

type Request struct {
	Adress         string
	Method         string
	MaxAttempts    int
	IdempotencyKey string
}

func ComputeSleepMs(n int, resp *http.Response) time.Duration {
	if resp != nil {
		if retryAfterHeader := resp.Header.Get("Retry-After"); retryAfterHeader != "" {
			if seconds, err := strconv.Atoi(retryAfterHeader); err == nil && seconds >= 0 {
				return time.Duration(seconds) * time.Second
			}
		}
	}

	exponent := max(0, n-2)
	calcInterval := min(2000, 200*(1<<exponent))
	return time.Duration(rand.Intn(calcInterval+1)) * time.Millisecond
}

func IsIdempotent(method string, idempotencyKey string) bool {
	switch method {
	case "GET", "HEAD", "PUT", "DELETE", "OPTIONS", "TRACE":
		return true
	default:
		return idempotencyKey != ""
	}
}

func Retry(n int, resp *http.Response) {
	timeSleep := ComputeSleepMs(n, resp)
	fmt.Printf("sleep_ms %d\n", timeSleep.Milliseconds())
	time.Sleep(timeSleep)
}

func main() {
	args := os.Args
	if len(args) < 2 {
		log.Fatal(Usage)
	}

	reqConfig := Request{
		Adress:         args[1],
		Method:         "GET",
		MaxAttempts:    5,
		IdempotencyKey: "",
	}

	for i := 2; i < len(args); i += 2 {
		if i+1 >= len(args) {
			log.Fatal(Usage)
		}
		flag, value := args[i], args[i+1]
		switch flag {
		case "--method":
			reqConfig.Method = value
		case "--max-attempts":
			number, err := strconv.Atoi(value)
			if err != nil {
				log.Fatal("--max-attempts int")
			}
			reqConfig.MaxAttempts = number
		case "--idempotency-key":
			reqConfig.IdempotencyKey = value
		default:
			log.Fatal(Usage)
		}
	}
	if !IsIdempotent(reqConfig.Method, reqConfig.IdempotencyKey) {
		reqConfig.MaxAttempts = 1
	}

	client := &http.Client{}

OuterLoop:
	for n := 1; n <= reqConfig.MaxAttempts; n++ {

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		httpReq, err := http.NewRequestWithContext(ctx, reqConfig.Method, reqConfig.Adress, nil)

		if err != nil {
			fmt.Printf("attempt %d error %v\n", n, err)
			if n < reqConfig.MaxAttempts {
				Retry(n+1, nil)
			}
			continue OuterLoop
		}

		if reqConfig.IdempotencyKey != "" {
			httpReq.Header.Set("Idempotency-Key", reqConfig.IdempotencyKey)
			httpReq.Header.Set("Content-Type", "application/json")
		}

		resp, err := client.Do(httpReq)
		if err != nil {
			fmt.Printf("attempt %d error %v\n", n, err)
			if n < reqConfig.MaxAttempts {
				Retry(n+1, nil)
			}
			continue OuterLoop
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		code := resp.StatusCode

		fmt.Printf("attempt %d status %d\n", n, code)

		switch {
		case code == 429 || code == 500 || code == 502 || code == 503 || code == 504:
			if n < reqConfig.MaxAttempts {
				Retry(n+1, resp)
			}
			continue OuterLoop
		case code >= 200 && code <= 399:
			fmt.Printf("result success attempts %d\n", n)
			os.Exit(0)
		default:
			fmt.Printf("result failure attempts %d\n", n)
			os.Exit(1)
		}
	}
	fmt.Printf("result failure attempts %d\n", reqConfig.MaxAttempts)
	os.Exit(1)
}
