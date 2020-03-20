package main

import (
  "fmt"
  "os"
  "log"
  "bytes"
  "net/http"
  "strings"
  "io"

  "github.com/gorilla/mux"
  "github.com/joho/godotenv"
)

var API_TOKEN string
const SERVICE_URL = "https://slack.com/api/"

func MapToString(m map[string]string) string {
    b := new(bytes.Buffer)
    fmt.Fprintf(b, "{")
    for key, value := range m {
        fmt.Fprintf(b, "\"%s\":\"%s\",", key, value)
    }
    if len(m) > 0 {
      b.Truncate(b.Len() - 2)
    } 
    fmt.Fprintf(b, "}")
    return b.String()
}

func CaptureResponseBody(r io.ReadCloser) string {
  builder := strings.Builder{}

  for {
    bytes := make([]byte, 256)
    length, err := r.Read(bytes)
    builder.Write(bytes[:length])
    if err != nil {
      break
    }
  }

  r.Close()
  return builder.String()
}

func PerformPost(url string, headers map[string]string, body string, includeAuth bool) (res *http.Response, err error) {
  url = fmt.Sprint(SERVICE_URL, url)
  req, err := http.NewRequest("POST", url, strings.NewReader(body))
  if err != nil {
    return nil, err
  } else {
    log.Println("Pre-request")
  }

  if includeAuth {
    authHeader := fmt.Sprintf("Bearer %s", API_TOKEN)
    req.Header.Add("Authorization", authHeader)
  }
  req.Header.Add("Content-Type", "application/json")
  req.Header.Add("charset", "utf-8")

  for key, value := range headers {
    req.Header.Add(key, value)
  }

  log.Printf("Performing POST for URL: %s\n", url)
  client := &http.Client{}
  defer log.Println("Post-request")
  return client.Do(req)
}

func SendMessage(message, channelId, thread string) {
  ret := make(map[string]string)
  ret["text"] = message
  if thread == "" {
    ret["thread_ts"] = thread
  }
  ret["channel"] = channelId

  req, err := http.NewRequest("POST", "chat.postMessage", strings.NewReader(MapToString(ret)))

  if err != nil {
    log.Fatal(err)
  } else {
    log.Println(req)
  }

  api_token := os.Getenv("API_TOKEN")
  authHeader := fmt.Sprintf("Bearer %s", api_token)
  req.Header.Add("Authorization", authHeader)
  client := &http.Client{}
  res, err := client.Do(req)
  if err != nil {
    log.Fatal(err)
  } else {
    log.Println(res)
  }
}

func PerformCheckin(w http.ResponseWriter, r *http.Request) {
  fmt.Println("Hello World")
  w.Write([]byte("Test"))
  fmt.Println(r.GetBody)
  SendMessage("hello", "D010AHVKFA9", "")
}

func TestSlack(error bool, message string) {
  var arg string
  if error {
    fmt.Printf("Error testing %s", message)
    arg = fmt.Sprintf("test_error=%s", message)
  } else {
    fmt.Printf("Testing %s", message)
    arg = fmt.Sprintf("test_message=%s", message)
  }
  url := fmt.Sprintf("api.test?%s", arg)
  res, err := PerformPost(url, nil, "", false)

  if err != nil {
    log.Fatal(err)
  } else {
    log.Printf("Status: %s", res.Status)
    log.Printf(CaptureResponseBody(res.Body))
  }
}

func TestSuccess(w http.ResponseWriter, r *http.Request) {
  TestSlack(false, r.URL.Path)
  w.Write([]byte("Tested Success"))
}

func TestError(w http.ResponseWriter, r *http.Request) {
  TestSlack(true, r.URL.Path)
  w.Write([]byte("Tested Error"))
}

func main() {
  err := godotenv.Load()
  if err != nil {
    log.Fatal(err)
  }
  port := os.Getenv("PORT")
  API_TOKEN = os.Getenv("API_TOKEN")
  if port == "" || API_TOKEN == "" {
    log.Fatal("PORT and API_TOKEN must be set")
  }

  log.Println("Server starting...")
 
  router := mux.NewRouter()

  router.HandleFunc("/", PerformCheckin)
  router.HandleFunc("/test", TestSuccess)
  router.HandleFunc("/testError", TestError)
  log.Fatal(http.ListenAndServe(port, router))
}
