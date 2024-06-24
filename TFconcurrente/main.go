package main

import (
	"encoding/csv"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// Struct para los vectores
type Vector struct {
	Data []float64
}

// Función para calcular la distancia euclidiana entre vectores
func squaredDistance(p1, p2 *Vector) float64 {
	var sum float64
	for i := 0; i < len(p1.Data); i++ {
		sum += (p1.Data[i] - p2.Data[i]) * (p1.Data[i] - p2.Data[i])
	}
	return sum
}

// Función para inicializar centroides de manera aleatoria
func initializeCentroids(data [][]float64, k int) [][]float64 {
	nSamples := len(data)
	centroids := make([][]float64, k)
	for i := 0; i < k; i++ {
		idx := rand.Intn(nSamples)
		centroids[i] = make([]float64, len(data[0]))
		copy(centroids[i], data[idx])
	}
	return centroids
}

// Función para calcular la media de vectores
func mean(vectors []*Vector) *Vector {
	n := len(vectors)
	meanVector := make([]float64, len(vectors[0].Data))
	for _, v := range vectors {
		for i := range v.Data {
			meanVector[i] += v.Data[i] / float64(n)
		}
	}
	return &Vector{meanVector}
}

// Función de K-means para calcular centroides y asignaciones
func kMeans(data [][]float64, k int, maxIterations int) ([][]float64, []int, error) {
	nSamples := len(data)
	centroids := initializeCentroids(data, k)
	assignments := make([]int, nSamples)
	vectors := make([]*Vector, nSamples)
	for i := range data {
		vectors[i] = &Vector{data[i]}
	}

	for iter := 0; iter < maxIterations; iter++ {
		assignChan := make(chan struct{ index, assignment int }, nSamples)
		updateChan := make(chan struct {
			index    int
			centroid []float64
		}, k)
		doneChan := make(chan bool)

		// Asignar puntos a los centroides más cercanos en paralelo
		go func() {
			var wg sync.WaitGroup
			wg.Add(nSamples)
			for i := 0; i < nSamples; i++ {
				go func(i int) {
					defer wg.Done()
					minDist := squaredDistance(vectors[i], &Vector{centroids[0]})
					assignment := 0
					for j := 1; j < k; j++ {
						dist := squaredDistance(vectors[i], &Vector{centroids[j]})
						if dist < minDist {
							minDist = dist
							assignment = j
						}
					}
					assignChan <- struct{ index, assignment int }{i, assignment}
				}(i)
			}
			wg.Wait()
			close(assignChan)
		}()

		go func() {
			for assignment := range assignChan {
				assignments[assignment.index] = assignment.assignment
			}
			doneChan <- true
		}()

		<-doneChan

		// Actualizar centroides en paralelo
		clusters := make([][]*Vector, k)
		for i := range clusters {
			clusters[i] = make([]*Vector, 0)
		}
		for i, idx := range assignments {
			clusters[idx] = append(clusters[idx], vectors[i])
		}

		go func() {
			var wg sync.WaitGroup
			wg.Add(k)
			for j := 0; j < k; j++ {
				go func(j int) {
					defer wg.Done()
					centroid := mean(clusters[j]).Data
					updateChan <- struct {
						index    int
						centroid []float64
					}{j, centroid}
				}(j)
			}
			wg.Wait()
			close(updateChan)
		}()

		go func() {
			for update := range updateChan {
				centroids[update.index] = update.centroid
			}
			doneChan <- true
		}()

		<-doneChan
	}

	return centroids, assignments, nil
}

// Función para guardar resultados en archivo CSV
func saveResults(filename string, ids []string, data [][]float64, assignments []int, centroids [][]float64) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"ID", "Frecuencia", "GastoT", "DiasSinCompra", "VariedadDeProductos", "Cluster"}
	writer.Write(header)

	for i := range ids {
		record := make([]string, len(data[i])+2)
		record[0] = ids[i]
		for j := range data[i] {
			record[j+1] = fmt.Sprintf("%.2f", data[i][j])
		}
		record[len(data[i])+1] = strconv.Itoa(assignments[i])
		writer.Write(record)
	}

	// Escribir los centroides al final del archivo
	writer.Write([]string{"Centroids:"})
	for _, centroid := range centroids {
		centroidRecord := make([]string, len(centroid))
		for i := range centroid {
			centroidRecord[i] = fmt.Sprintf("%.2f", centroid[i])
		}
		writer.Write(centroidRecord)
	}

	return nil
}

func main() {
	rand.Seed(time.Now().UnixNano())

	// Configuración inicial
	k := 3               // Número de clusters
	maxIterations := 100 // Número máximo de iteraciones para K-means

	// Configurar el servidor web Echo
	e := echo.New()

	// Manejador para cargar página HTML estática
	e.Static("/", "static")

	// Manejador para procesar la carga de datos CSV y calcular K-means
	e.POST("/upload", func(c echo.Context) error {
		// Procesar el archivo CSV cargado desde el formulario HTML
		file, err := c.FormFile("csvfile")
		if err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		defer src.Close()

		// Leer el archivo CSV
		reader := csv.NewReader(src)
		records, err := reader.ReadAll()
		if err != nil {
			return err
		}

		// Procesar los datos del archivo CSV
		data := make([][]float64, len(records)-1)
		ids := make([]string, len(records)-1)
		for i := 1; i < len(records); i++ {
			data[i-1] = make([]float64, len(records[i])-1)
			ids[i-1] = records[i][0]
			for j := 1; j < len(records[i]); j++ {
				data[i-1][j-1], err = strconv.ParseFloat(records[i][j], 64)
				if err != nil {
					return err
				}
			}
		}

		// Ejecutar K-means con los datos del archivo CSV cargado
		centroids, assignments, err := kMeans(data, k, maxIterations)
		if err != nil {
			return err
		}

		// Guardar resultados en archivo CSV
		outputFilename := "resultados.csv"
		err = saveResults(outputFilename, ids, data, assignments, centroids)
		if err != nil {
			return err
		}

		// Enviar archivo CSV resultante para descarga
		c.Response().Header().Set("Content-Disposition", "attachment; filename=resultados.csv")
		c.Response().Header().Set("Content-Type", "text/csv")
		return c.File(outputFilename)
	})

	// Iniciar el servidor web
	e.Logger.Fatal(e.Start(":8080"))
}
