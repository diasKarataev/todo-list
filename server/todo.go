package main

import (
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var limiter = rate.NewLimiter(1, 1) // Rate limit of 1 request

const (
	dsn = "host=localhost user=postgres password=Infinitive dbname=go-todo-app port=5432 sslmode=disable"
)

var db *gorm.DB
var log *logrus.Logger

type Task struct {
	ID          uint      `gorm:"primaryKey"`
	Name        string    `json:"name"`
	Details     string    `json:"details"`
	CreatedDate time.Time `json:"createdDate"`
	HaveStar    bool      `json:"star" gorm:"default:false"`
	LastUpdated time.Time `json:"lastUpdated" gorm:"column:lastupdated"`
}

func (t *Task) ToggleHaveStar() {
	t.HaveStar = !t.HaveStar
	t.LastUpdated = time.Now()
}

func checkLimiter(c *gin.Context) {
	if !limiter.Allow() {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests"})
		return
	}
}

func main() {
	log = logrus.New()
	var err error
	log.SetFormatter(&logrus.JSONFormatter{})

	file, err := os.OpenFile("logfile.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		log.SetOutput(file)
		log.Info("Log file opened successfully")
	} else {
		log.WithError(err).Fatal("Failed to open log file")
	}
	defer file.Close()

	log.WithFields(logrus.Fields{
		"action": "start",
		"status": "success",
	}).Info("Application started successfully")

	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	db.AutoMigrate(&Task{})

	r := gin.Default()

	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"http://localhost:3000"}
	r.Use(cors.New(config))

	r.GET("/tasks", GetTasks)
	r.GET("/tasks/:id", GetTask)
	r.POST("/tasks", CreateTask)
	r.PUT("/tasks/:id", UpdateTask)
	r.DELETE("/tasks/:id", DeleteTask)
	r.PATCH("/tasks/:id/toggle-star", ToggleStarTask)

	log.Println("Сервер запущен на порту :8000")
	log.Fatal(http.ListenAndServe(":8000", r))
}

func GetTasks(c *gin.Context) {
	checkLimiter(c)
	if c.IsAborted() {
		return
	}
	var tasks []Task

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	nameFilter := c.Query("name")
	detailsFilter := c.Query("details")
	starFilter, _ := strconv.ParseBool(c.Query("star"))
	sortField := c.Query("sortField")
	sortOrder := c.DefaultQuery("sortOrder", "asc")

	if sortField == "" {
		sortField = "ID"
	}

	offset := (page - 1) * pageSize

	query := db.Offset(offset).Limit(pageSize)

	if nameFilter != "" {
		query = query.Where("name LIKE ?", "%"+nameFilter+"%")
	}

	if detailsFilter != "" {
		query = query.Where("details LIKE ?", "%"+detailsFilter+"%")
	}

	if starFilter {
		query = query.Where("have_star = ?", true)
	}

	orderClause := sortField + " " + sortOrder
	query = query.Order(orderClause)

	if err := query.Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения задач"})
		return
	}
	log.WithFields(logrus.Fields{
		"action":     "getTasks",
		"page":       page,
		"pageSize":   pageSize,
		"nameFilter": nameFilter,
		// ... (добавьте другие поля лога по вашему усмотрению)
	}).Info("GetTasks executed successfully")

	c.JSON(http.StatusOK, tasks)

}

func GetTask(c *gin.Context) {
	checkLimiter(c)
	if c.IsAborted() {
		return
	}
	var task Task
	id := c.Param("id")
	if err := db.First(&task, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Задача не найдена"})
		return
	}

	log.WithFields(logrus.Fields{
		"action": "getTasks",
	}).Info("GetTask executed successfully")

	c.JSON(http.StatusOK, task)
}

func CreateTask(c *gin.Context) {
	checkLimiter(c)
	if c.IsAborted() {
		return
	}
	var newTask Task
	if err := c.BindJSON(&newTask); err != nil {
		log.WithFields(logrus.Fields{
			"action": "createTask",
			"error":  err.Error(),
		}).Error("Error binding JSON for creating task")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Ошибка входных данных"})
		return
	}

	newTask.CreatedDate = time.Now()
	newTask.LastUpdated = newTask.CreatedDate
	newTask.HaveStar = false

	if err := db.Create(&newTask).Error; err != nil {
		log.WithFields(logrus.Fields{
			"action": "createTask",
			"error":  err.Error(),
		}).Error("Error creating task in the database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания задачи"})
		return
	}

	log.WithFields(logrus.Fields{
		"action": "createTask",
		"taskID": newTask.ID,
	}).Info("Task created successfully")

	c.JSON(http.StatusCreated, newTask)
}

func UpdateTask(c *gin.Context) {
	checkLimiter(c)
	if c.IsAborted() {
		return
	}
	var updatedTask Task
	id := c.Param("id")

	if err := db.First(&updatedTask, id).Error; err != nil {
		log.WithFields(logrus.Fields{
			"action": "updateTask",
			"error":  err.Error(),
		}).Error("Error retrieving task for update")
		c.JSON(http.StatusNotFound, gin.H{"error": "Задача не найдена"})
		return
	}

	if err := c.BindJSON(&updatedTask); err != nil {
		log.WithFields(logrus.Fields{
			"action": "updateTask",
			"error":  err.Error(),
		}).Error("Error binding JSON for updating task")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Ошибка входных данных"})
		return
	}

	updatedTask.LastUpdated = time.Now()

	if err := db.Save(&updatedTask).Error; err != nil {
		log.WithFields(logrus.Fields{
			"action": "updateTask",
			"error":  err.Error(),
		}).Error("Error updating task in the database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления задачи"})
		return
	}

	log.WithFields(logrus.Fields{
		"action": "updateTask",
		"taskID": updatedTask.ID,
	}).Info("Task updated successfully")

	c.JSON(http.StatusOK, updatedTask)
}

func DeleteTask(c *gin.Context) {
	checkLimiter(c)
	if c.IsAborted() {
		return
	}
	var task Task
	id := c.Param("id")

	if err := db.First(&task, id).Error; err != nil {
		log.WithFields(logrus.Fields{
			"action": "deleteTask",
			"error":  err.Error(),
		}).Error("Error retrieving task for delete")
		c.JSON(http.StatusNotFound, gin.H{"error": "Задача не найдена"})
		return
	}

	if err := db.Delete(&task).Error; err != nil {
		log.WithFields(logrus.Fields{
			"action": "deleteTask",
			"error":  err.Error(),
		}).Error("Error deleting task from the database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления задачи"})
		return
	}

	log.WithFields(logrus.Fields{
		"action": "deleteTask",
		"taskID": task.ID,
	}).Info("Task deleted successfully")

	c.JSON(http.StatusOK, gin.H{"message": "Задача успешно удалена"})
}

func ToggleStarTask(c *gin.Context) {
	checkLimiter(c)
	if c.IsAborted() {
		return
	}
	var task Task
	id := c.Param("id")

	if err := db.First(&task, id).Error; err != nil {
		log.WithFields(logrus.Fields{
			"action": "toggleStarTask",
			"error":  err.Error(),
		}).Error("Error retrieving task for toggle star")
		c.JSON(http.StatusNotFound, gin.H{"error": "Задача не найдена"})
		return
	}

	task.ToggleHaveStar()

	if err := db.Save(&task).Error; err != nil {
		log.WithFields(logrus.Fields{
			"action": "toggleStarTask",
			"error":  err.Error(),
		}).Error("Error updating task for toggle star")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления задачи"})
		return
	}

	log.WithFields(logrus.Fields{
		"action":      "toggleStarTask",
		"taskID":      task.ID,
		"haveStar":    task.HaveStar,
		"lastUpdated": task.LastUpdated,
	}).Info("Task star status toggled successfully")

	c.JSON(http.StatusOK, gin.H{"haveStar": task.HaveStar, "lastUpdated": task.LastUpdated})
}
