package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

type Bot struct {
	Left  int
	Right int
}

type BotResult struct {
	Data int
}

func Plus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var b Bot
	err := json.NewDecoder(r.Body).Decode(&b)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result := BotResult{
		Data: b.Left + b.Right,
	}
	jsonResp, _ := json.Marshal(result)
	log.Printf("The plus result is %+v", result.Data)
	w.Write(jsonResp)
}

func Minus(w http.ResponseWriter, r *http.Request) {
	var b Bot

	err := json.NewDecoder(r.Body).Decode(&b)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result := b.Left - b.Right
	log.Println(result)
	log.Printf("The minus result is %+v", result)

	w.Write([]byte(strconv.Itoa(result)))
}

func Times(w http.ResponseWriter, r *http.Request) {
	var b Bot

	err := json.NewDecoder(r.Body).Decode(&b)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result := b.Left * b.Right
	log.Printf("The time result is %+v", result)
	w.Write([]byte(strconv.Itoa(result)))
}

func Divide(w http.ResponseWriter, r *http.Request) {
	var b Bot

	err := json.NewDecoder(r.Body).Decode(&b)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result := b.Left / b.Right
	log.Printf("The divide result is %+v", result)
	w.Write([]byte(strconv.Itoa(result)))
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/plus", Plus)
	mux.HandleFunc("/minus", Minus)
	mux.HandleFunc("/times", Times)
	mux.HandleFunc("/divide", Divide)

	http.ListenAndServe(":8008", mux)
}
