package queue

import (
	"relay/pkg/models"
)

type Queue interface {
	Enqueue(message *models.Message) error
	Dequeue(batchSize int) ([]*models.Message, error)
	UpdateStatus(id string, status models.MessageStatus, err error) error
	Get(id string) (*models.Message, error)
	Remove(id string) error
	Close() error
	GetSentCountsByWorkspaceAndSender() (map[string]map[string]int, error)
}
