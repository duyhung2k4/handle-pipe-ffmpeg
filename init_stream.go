package sendblobhandle

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (h *sendblobHandle) InitStream(ctx *gin.Context) {
	// Nâng cấp kết nối WebSocket
	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		fmt.Println("Error upgrading connection:", err)
		return
	}
	defer conn.Close()

	// Tạo pipe giao tiếp với ffmpeg
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()

	// Cấu hình ffmpeg để xử lý liên tục
	cmd := exec.Command("ffmpeg",
		"-f", "webm", // Định dạng đầu vào là WebM
		"-i", "pipe:0", // Nhận từ stdin
		"-f", "webm", // Định dạng đầu ra là WebM
		"-vcodec", "libvpx", // Bộ mã hóa video VP8
		"-s", "256x144", // Chuyển đổi độ phân giải video xuống 480p (854x480)
		"-acodec", "libopus", // Bộ mã hóa âm thanh Opus
		"-b:a", "128k", // Giữ bitrate âm thanh ở mức 64 kbps (âm thanh giữ nguyên chất lượng)
		"pipe:1", // Ghi ra stdout
	)

	cmd.Stdin = inputReader
	cmd.Stdout = outputWriter
	cmd.Stderr = os.Stderr

	// Chạy ffmpeg trong goroutine (chỉ khởi tạo một lần)
	err = cmd.Start()
	if err != nil {
		log.Fatalf("Lỗi khi khởi động ffmpeg: %v", err)
	}
	defer cmd.Wait()

	// Goroutine đọc dữ liệu từ ffmpeg và gửi qua WebSocket
	go func() {
		defer outputReader.Close()
		buffer := make([]byte, 4096)
		for {
			n, err := outputReader.Read(buffer)
			if err == io.EOF {
				log.Println("Kết thúc luồng đầu ra từ ffmpeg")
				break
			}
			if err != nil {
				log.Printf("Lỗi khi đọc từ ffmpeg: %v", err)
				break
			}

			// Gửi dữ liệu xử lý qua WebSocket
			// Sử dụng BinaryMessage nếu cần
			err = conn.WriteMessage(websocket.BinaryMessage, buffer[:n])
			if err != nil {
				log.Printf("Lỗi khi gửi dữ liệu qua WebSocket: %v", err)
				break
			}
		}
	}()

	// Nhận dữ liệu liên tục từ WebSocket và gửi vào ffmpeg
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Lỗi khi nhận tin nhắn từ WebSocket: %v", err)
			break
		}

		// Kiểm tra kích thước và ghi dữ liệu vào pipe
		log.Println("send: ", len(data))

		// Kiểm tra xem dữ liệu có hợp lệ trước khi ghi
		if len(data) > 0 {
			_, err = inputWriter.Write(data)
			if err != nil {
				log.Printf("Lỗi khi ghi dữ liệu vào ffmpeg: %v", err)
				break
			}
		}
	}
}
