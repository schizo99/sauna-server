package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/dhowden/raspicam"
	"github.com/op/go-logging"
	"github.com/otiai10/gosseract"
	"gocv.io/x/gocv"
)

var errorCount = 0
var startCounter = 6
var lastTemp = 0
var sentZero = time.Now().Unix()
var path = "./test.jpg"
var config iftttConfig
var alertSent = false

var log = logging.MustGetLogger("sauna")

// Example format string. Everything except the message has a custom color
// which is dependent on the log level. Many fields have a custom output
// formatting too, eg. the time returns the hour down to the milli second.
var format = logging.MustStringFormatter(
	`%{color}%{level:.8s} %{shortfunc} ▶ %{color:reset} %{message}`,
)

//iftttConfig holds the configuration
type iftttConfig struct {
	BackendURL string
	IftttURL   string
	LogLevel   string
}

//Temp is used to json conversion
type Temp struct {
	Temp string `json:"temp"`
}

//IFTTT is used for json conversion
type IFTTT struct {
	Value1 string `json:"value1"`
}

func (c *iftttConfig) readConfig() {
	if _, err := toml.DecodeFile("config.toml", &c); err != nil {
		log.Fatal(err)
	}
}

func takePicture() bool {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("create file: %v", err)
		return false
	}
	defer f.Close()

	s := raspicam.NewStill()
	s.Camera.ExposureCompensation = -9
	s.Camera.Rotation = 180
	errCh := make(chan error)
	var error error
	go func() {
		for x := range errCh {
			log.Critical("Unable to take Picture: ", x)
			error = x
		}
	}()
	log.Critical("Capturing image...")
	raspicam.Capture(s, f, errCh)
	if error != nil {
		return false
	}
	return true
}

func readImage() (image.Image, int, int, bool) {
	reader, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer reader.Close()

	imgCfg, _, err := image.DecodeConfig(reader)

	if err != nil {
		log.Warning("Unable to Decode image", err)
		return nil, 0, 0, false
	}

	width := imgCfg.Width
	height := imgCfg.Height

	// we need to reset the io.Reader again for image.Decode() function below to work
	// otherwise we will  - panic: runtime error: invalid memory address or nil pointer dereference
	// there is no build in rewind for io.Reader, use Seek(0,0)
	reader.Seek(0, 0)

	// get the image
	img, _, err := image.Decode(reader)
	counter := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r>>8 > 100 && g>>8 < 50 && b>>8 < 50 {
				counter++
			}
			//fmt.Printf("[X : %d Y : %v] R : %v, G : %v, B : %v  \n", x, y, r>>8, g>>8, b>>8)
		}
	}
	log.Debugf("There are %v red pixels\n", counter)
	if counter > 150 {
		return img, width, height, true
	}
	return img, width, height, false
}
func init() {
	// damn important or else At(), Bounds() functions will
	// caused memory pointer error!!
	image.RegisterFormat("jpeg", "jpeg", jpeg.Decode, jpeg.DecodeConfig)
}

func convertImage(img image.Image, width, height int) gocv.Mat {
	color, err := gocv.ImageToMatRGBA(img)
	if err != nil {
		log.Warning("Error converting image")
	}
	gray := gocv.NewMat()
	gocv.CvtColor(color, &gray, gocv.ColorRGBAToGray)
	return gray
}

func findCountours(img gocv.Mat, edge float32) gocv.Mat {
	blurred := gocv.NewMat()
	gocv.GaussianBlur(img, &blurred, image.Point{5, 5}, 0, 0, gocv.BorderDefault)
	edges := gocv.NewMat()
	gocv.Canny(blurred, &edges, 170.0, 255.0)
	contours := gocv.FindContours(edges, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	gocv.DrawContours(&img, contours, len(contours)-1, color.RGBA{255, 255, 255, 255}, 3)
	//data := img.ToBytes()
	// pixel values for each channel - we know this is a BGR image
	for x := 0; x < img.Rows(); x++ {
		for y := 0; y < img.Cols(); y++ {
			g := img.GetUCharAt(x, y)
			if g < 240 {
				img.SetUCharAt(x, y, 0)
			}
		}
	}
	return img
}

func ocrImage(client gosseract.Client, img image.Image, w, h int) string {
	convertImage := convertImage(img, w, h)
	blurred := findCountours(convertImage, 70.0)
	b, _ := gocv.IMEncode(gocv.FileExt(".jpg"), blurred)
	client.SetImageFromBytes(b)
	client.SetLanguage("lets")
	text, _ := client.Text()
	return text
}

func sendPost(jsonStr interface{}, url string) {
	log.Critical(url)
	b, err := json.Marshal(jsonStr)
	if err != nil {
		log.Error("error marshalling")
	} else {
		log.Debugf("Sending temp: %s to backend\n", string(b))
	}
	req, err := http.Post(url, "application/json", bytes.NewBuffer(b))
	log.Debug(url)
	if err != nil {
		log.Critical("Failed to send to backend", err)
	}
	defer req.Body.Close()

}

//sendTemp POSTs result to backend server
func sendTemp(temp string) {
	jsonStr := Temp{Temp: temp}
	sendPost(jsonStr, config.BackendURL)
}

func sendAlert(temp string) {
	log.Notice("Sending temp to IFTTT:", temp)
	jsonStr := IFTTT{Value1: temp}
	sendPost(jsonStr, config.IftttURL)
}

func checkTempLevel(temp int) {
	tempStr := strconv.Itoa(temp)

	switch {
	case temp > 0 && startCounter > 0:
		log.Debug("Sauna just started. Sleep before we start sending.")
		startCounter--
	case temp > 100:
		sendAlert(tempStr)
		alertSent = true
		fallthrough
	case temp > 0:
		sendTemp(tempStr)
		lastTemp = temp
	}

}

func checkError() {
	switch {
	case errorCount == 100 && lastTemp != 0: //Assume sauna been off for more than 20 minutes
		sendTemp("0")
		sentZero = time.Now().Unix()
		lastTemp = 0
		startCounter = 6
	case errorCount > 100:
		if int64(time.Now().Unix())-sentZero > 21600 {
			sendTemp("0")
			sentZero = time.Now().Unix()
		}
		errorCount = 0
	default:
		errorCount++
	}
}

func setupLogging() {
	config.readConfig()
	level, err := logging.LogLevel(config.LogLevel)
	if err != nil {
		fmt.Println("Unable to get LogLevel from config file, setting to INFO")
		level = logging.INFO
	}
	logging.SetFormatter(format)
	logging.SetLevel(level, "sauna")
}

func main() {
	client := gosseract.NewClient()
	defer client.Close()
	setupLogging()

MAIN:
	ok := takePicture()
	img, w, h, ok := readImage()
	if !ok {
		checkError()
		time.Sleep(5 * time.Second)
		goto MAIN
	}
	temp := ocrImage(*client, img, w, h)
	tempInt, err := strconv.Atoi(temp)
	if err != nil {
		log.Critical("Unable to determine temp from picture: ", temp)
		checkError()
		time.Sleep(5 * time.Second)
		goto MAIN
	}
	errorCount = 0
	log.Info("The temp is: ", temp, errorCount)
	checkTempLevel(tempInt)
	time.Sleep(5 * time.Second)
	goto MAIN
}
