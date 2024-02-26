package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
)

type Client struct {
	ID             int
	Limit          int
	InitialBalance int
	ActualBalance  int
}

type TransactionIn struct {
	Value       int    `json:"valor"`
	TxType      string `json:"tipo"`
	Description string `json:"descricao"`
}

func main() {
	dbHost := os.Getenv("DB_HOST")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASS")
	dbName := os.Getenv("DB_NAME")
	dbPort, _ := strconv.Atoi(os.Getenv("DB_PORT"))

	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", dbHost, dbPort, dbUser, dbPass, dbName)

	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	for {
		if err := db.Ping(context.Background()); err != nil {
			log.Println("Trying to connect in db, wait 1 second...")
			time.Sleep(1 * time.Second)
			break
		}
	}

	log.Println("DB connected")

	r := gin.Default()

	r.POST("/clientes/:client_id/transacoes", func(ctx *gin.Context) {
		clientIDParam := ctx.Param("client_id")
		clientID, err := strconv.Atoi(clientIDParam)
		if err != nil {
			ctx.Status(http.StatusNotFound)
			return
		}

		txIn := TransactionIn{}
		if err := ctx.BindJSON(&txIn); err != nil {
			ctx.Status(http.StatusUnprocessableEntity)
			return
		}

		if len(txIn.Description) > 10 || txIn.Description == "" {
			ctx.Status(http.StatusUnprocessableEntity)
			return
		}

		stmt := "SELECT * FROM clients WHERE id = $1"

		client := Client{}

		if err := db.QueryRow(context.Background(), stmt, clientID).Scan(&client.ID, &client.Limit, &client.InitialBalance, &client.ActualBalance); err != nil {
			ctx.Status(http.StatusNotFound)
			return
		}

		newBalance := 0

		switch txIn.TxType {
		case "d":
			if client.ActualBalance+client.Limit < txIn.Value {
				ctx.Status(http.StatusUnprocessableEntity)
				return
			}
			newBalance = client.ActualBalance - txIn.Value
		case "c":
			newBalance = client.ActualBalance + txIn.Value
		default:
			ctx.Status(http.StatusUnprocessableEntity)
			return
		}

		client.ActualBalance = newBalance

		stmt = "UPDATE clients SET actual_balance = $1 WHERE id = $2"

		if _, err := db.Exec(context.Background(), stmt, client.ActualBalance, client.ID); err != nil {
			ctx.Status(http.StatusUnprocessableEntity)
			return
		}

		stmt = "INSERT INTO transactions (value, transaction_type, description, completed_at, client_id) VALUES ($1, $2, $3, $4, $5)"

		if _, err := db.Exec(context.Background(), stmt, txIn.Value, txIn.TxType, txIn.Description, time.Now(), clientID); err != nil {
			ctx.Status(http.StatusInternalServerError)
			return
		}

		ctx.JSON(http.StatusOK, map[string]int{
			"saldo":  client.ActualBalance,
			"limite": client.Limit,
		})
	})

	r.GET("/clientes/:client_id/extrato", func(ctx *gin.Context) {
		clientIDParam := ctx.Param("client_id")
		clientID, err := strconv.Atoi(clientIDParam)
		if err != nil {
			ctx.Status(http.StatusNotFound)
			return
		}

		stmt := "SELECT * FROM clients WHERE id = $1"

		client := Client{}

		if err := db.QueryRow(context.Background(), stmt, clientID).Scan(&client.ID, &client.Limit, &client.InitialBalance, &client.ActualBalance); err != nil {
			ctx.Status(http.StatusNotFound)
			return
		}

		stmt = "SELECT value, transaction_type, description, completed_at FROM transactions WHERE client_id = $1 ORDER BY completed_at DESC LIMIT 10"

		txs := []map[string]interface{}{}

		rows, err := db.Query(context.Background(), stmt, clientID)
		if err != nil {
			ctx.Status(http.StatusInternalServerError)
			return
		}

		for rows.Next() {
			var value int
			var txType string
			var description string
			var completedAt time.Time

			if err := rows.Scan(&value, &txType, &description, &completedAt); err != nil {
				ctx.Status(http.StatusInternalServerError)
				return
			}
			txs = append(txs, map[string]interface{}{
				"valor":        value,
				"tipo":         txType,
				"descricao":    description,
				"realizada_em": completedAt,
			})
		}

		ctx.JSON(http.StatusOK, map[string]interface{}{
			"saldo": map[string]interface{}{
				"total":        client.ActualBalance,
				"data_extrato": time.Now(),
				"limite":       client.Limit,
			},
			"ultimas_transacoes": txs,
		})
	})

	fmt.Println("Server running at 0.0.0.0:8080")

	r.Run(":8080")
}
