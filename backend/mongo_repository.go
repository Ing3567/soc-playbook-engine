package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type storedImageRecord struct {
	meta   NodeImage
	sha256 string
	data   []byte
}

type inMemoryPlaybookStore struct {
	mu           sync.RWMutex
	playbooks    map[string]PlaybookDocument
	imagesByID   map[string]storedImageRecord
	imageIDBySHA map[string]string
	seq          int
}

type PlaybookRepository struct {
	collection  *mongo.Collection
	imageBucket *mongo.GridFSBucket
	mem         *inMemoryPlaybookStore
}

func NewPlaybookRepository(ctx context.Context, uri, dbName string) (*PlaybookRepository, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	db := client.Database(dbName)
	bucket := db.GridFSBucket(options.GridFSBucket().SetName("playbook_images"))
	_, _ = bucket.GetFilesCollection().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "metadata.sha256", Value: 1}},
	})
	return &PlaybookRepository{
		collection:  db.Collection("playbooks"),
		imageBucket: bucket,
	}, nil
}

func NewInMemoryPlaybookRepository() *PlaybookRepository {
	return &PlaybookRepository{
		mem: &inMemoryPlaybookStore{
			playbooks:    make(map[string]PlaybookDocument),
			imagesByID:   make(map[string]storedImageRecord),
			imageIDBySHA: make(map[string]string),
		},
	}
}

func (r *PlaybookRepository) ListAll(ctx context.Context) ([]PlaybookDocument, error) {
	if r != nil && r.mem != nil {
		r.mem.mu.RLock()
		defer r.mem.mu.RUnlock()
		items := make([]PlaybookDocument, 0, len(r.mem.playbooks))
		for _, doc := range r.mem.playbooks {
			items = append(items, doc)
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		})
		return items, nil
	}

	opts := options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}})
	cursor, err := r.collection.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	items := []PlaybookDocument{}
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *PlaybookRepository) SaveOne(ctx context.Context, doc PlaybookDocument) (PlaybookDocument, error) {
	if strings.TrimSpace(doc.Title) == "" {
		doc.Title = "Untitled SOP"
	}
	if strings.TrimSpace(doc.ID) == "" {
		doc.ID = fmt.Sprintf("sop_%d", time.Now().UnixNano())
	}
	doc.UpdatedAt = time.Now()

	if r != nil && r.mem != nil {
		r.mem.mu.Lock()
		defer r.mem.mu.Unlock()
		r.mem.playbooks[doc.ID] = doc
		return doc, nil
	}

	_, err := r.collection.ReplaceOne(ctx, bson.M{"_id": doc.ID}, doc, options.Replace().SetUpsert(true))
	return doc, err
}

func (r *PlaybookRepository) DeleteOne(ctx context.Context, id string) error {
	if r != nil && r.mem != nil {
		r.mem.mu.Lock()
		defer r.mem.mu.Unlock()
		doc, exists := r.mem.playbooks[id]
		delete(r.mem.playbooks, id)
		if exists {
			candidates := collectPlaybookImageIDs(doc)
			for _, imgID := range candidates {
				stillUsed := false
				for _, other := range r.mem.playbooks {
					if playbookReferencesImage(other, imgID) {
						stillUsed = true
						break
					}
				}
				if !stillUsed {
					if rec, ok := r.mem.imagesByID[imgID]; ok {
						delete(r.mem.imageIDBySHA, rec.sha256)
						delete(r.mem.imagesByID, imgID)
					}
				}
			}
		}
		return nil
	}

	var existing PlaybookDocument
	_ = r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&existing)
	candidateIDs := collectPlaybookImageIDs(existing)

	if _, err := r.collection.DeleteOne(ctx, bson.M{"_id": id}); err != nil {
		return err
	}

	if r.imageBucket != nil && len(candidateIDs) > 0 {
		for _, imgID := range candidateIDs {
			count, countErr := r.collection.CountDocuments(ctx, bson.M{"nodes.images.id": imgID})
			if countErr == nil && count == 0 {
				if objID, hexErr := bson.ObjectIDFromHex(imgID); hexErr == nil {
					_ = r.imageBucket.Delete(ctx, objID)
				} else {
					_ = r.imageBucket.Delete(ctx, imgID)
				}
			}
		}
	}

	return nil
}

func (r *PlaybookRepository) SaveImage(ctx context.Context, img ExtractedImage) (NodeImage, error) {
	if len(img.Data) == 0 {
		return NodeImage{}, errors.New("empty image data")
	}

	contentType := sanitizeImageContentType(img.ContentType, img.Filename, img.Data)
	filename := strings.TrimSpace(img.Filename)
	if filename == "" {
		filename = "image" + defaultExtensionForMIME(contentType)
	}
	caption := strings.TrimSpace(img.Caption)
	sum := sha256.Sum256(img.Data)
	hashHex := fmt.Sprintf("%x", sum)

	if r == nil || (r.imageBucket == nil && r.mem == nil) {
		shortID := hashHex[:24]
		return NodeImage{
			ID:          shortID,
			Filename:    filename,
			ContentType: contentType,
			URL:         "/api/images/" + shortID,
			Caption:     caption,
		}, nil
	}

	if r.mem != nil {
		r.mem.mu.Lock()
		defer r.mem.mu.Unlock()
		if existingID, ok := r.mem.imageIDBySHA[hashHex]; ok {
			if rec, found := r.mem.imagesByID[existingID]; found {
				out := rec.meta
				if caption != "" {
					out.Caption = caption
				}
				return out, nil
			}
		}
		r.mem.seq++
		idHex := fmt.Sprintf("%024x", r.mem.seq)
		meta := NodeImage{
			ID:          idHex,
			Filename:    filename,
			ContentType: contentType,
			URL:         "/api/images/" + idHex,
			Caption:     caption,
		}
		dataCopy := make([]byte, len(img.Data))
		copy(dataCopy, img.Data)
		r.mem.imagesByID[idHex] = storedImageRecord{
			meta:   meta,
			sha256: hashHex,
			data:   dataCopy,
		}
		r.mem.imageIDBySHA[hashHex] = idHex
		return meta, nil
	}

	var existingFile mongo.GridFSFile
	if err := r.imageBucket.GetFilesCollection().FindOne(ctx, bson.M{"metadata.sha256": hashHex}).Decode(&existingFile); err == nil {
		idHex := gridFSIDToHex(existingFile.ID)
		if idHex != "" {
			resolvedName := filename
			if resolvedName == "" {
				resolvedName = existingFile.Name
			}
			return NodeImage{
				ID:          idHex,
				Filename:    resolvedName,
				ContentType: contentType,
				URL:         "/api/images/" + idHex,
				Caption:     caption,
			}, nil
		}
	}

	metaDoc := bson.M{
		"content_type": contentType,
		"sha256":       hashHex,
		"caption":      caption,
		"size":         len(img.Data),
		"created_at":   time.Now().UTC(),
	}
	uploadOpts := options.GridFSUpload().SetMetadata(metaDoc)
	fileID, err := r.imageBucket.UploadFromStream(ctx, filename, bytes.NewReader(img.Data), uploadOpts)
	if err != nil {
		return NodeImage{}, err
	}
	idHex := fileID.Hex()
	return NodeImage{
		ID:          idHex,
		Filename:    filename,
		ContentType: contentType,
		URL:         "/api/images/" + idHex,
		Caption:     caption,
	}, nil
}

func (r *PlaybookRepository) GetImage(ctx context.Context, id string) ([]byte, NodeImage, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return nil, NodeImage{}, errors.New("empty image id")
	}

	if r != nil && r.mem != nil {
		r.mem.mu.RLock()
		defer r.mem.mu.RUnlock()
		rec, ok := r.mem.imagesByID[trimmedID]
		if !ok {
			return nil, NodeImage{}, errors.New("image not found")
		}
		dataCopy := make([]byte, len(rec.data))
		copy(dataCopy, rec.data)
		return dataCopy, rec.meta, nil
	}

	if r == nil || r.imageBucket == nil {
		return nil, NodeImage{}, errors.New("image bucket not configured")
	}

	var fileID any = trimmedID
	if objID, err := bson.ObjectIDFromHex(trimmedID); err == nil {
		fileID = objID
	}

	stream, err := r.imageBucket.OpenDownloadStream(ctx, fileID)
	if err != nil {
		return nil, NodeImage{}, err
	}
	defer stream.Close()

	gf := stream.GetFile()
	data, err := io.ReadAll(io.LimitReader(stream, 25<<20))
	if err != nil {
		return nil, NodeImage{}, err
	}

	var meta struct {
		ContentType string `bson:"content_type"`
		Caption     string `bson:"caption"`
	}
	if gf != nil && len(gf.Metadata) > 0 {
		_ = bson.Unmarshal(gf.Metadata, &meta)
	}

	filename := "image"
	if gf != nil && strings.TrimSpace(gf.Name) != "" {
		filename = gf.Name
	}
	contentType := sanitizeImageContentType(meta.ContentType, filename, data)

	return data, NodeImage{
		ID:          trimmedID,
		Filename:    filename,
		ContentType: contentType,
		URL:         "/api/images/" + trimmedID,
		Caption:     meta.Caption,
	}, nil
}

func (r *PlaybookRepository) GetImageMeta(ctx context.Context, id string) (NodeImage, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return NodeImage{}, errors.New("empty image id")
	}

	if r != nil && r.mem != nil {
		r.mem.mu.RLock()
		defer r.mem.mu.RUnlock()
		if rec, ok := r.mem.imagesByID[trimmedID]; ok {
			return rec.meta, nil
		}
		return NodeImage{}, errors.New("image not found")
	}

	if r == nil || r.imageBucket == nil {
		return NodeImage{}, errors.New("image bucket not configured")
	}

	var fileID any = trimmedID
	if objID, err := bson.ObjectIDFromHex(trimmedID); err == nil {
		fileID = objID
	}

	var gf mongo.GridFSFile
	if err := r.imageBucket.GetFilesCollection().FindOne(ctx, bson.M{"_id": fileID}).Decode(&gf); err != nil {
		return NodeImage{}, err
	}

	var meta struct {
		ContentType string `bson:"content_type"`
		Caption     string `bson:"caption"`
	}
	if len(gf.Metadata) > 0 {
		_ = bson.Unmarshal(gf.Metadata, &meta)
	}
	filename := gf.Name
	if filename == "" {
		filename = "image.png"
	}
	contentType := sanitizeImageContentType(meta.ContentType, filename, nil)
	return NodeImage{
		ID:          trimmedID,
		Filename:    filename,
		ContentType: contentType,
		URL:         "/api/images/" + trimmedID,
		Caption:     meta.Caption,
	}, nil
}

func collectPlaybookImageIDs(doc PlaybookDocument) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, n := range doc.Nodes {
		for _, img := range n.Images {
			id := strings.TrimSpace(img.ID)
			if id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func playbookReferencesImage(doc PlaybookDocument, imageID string) bool {
	for _, n := range doc.Nodes {
		for _, img := range n.Images {
			if img.ID == imageID {
				return true
			}
		}
	}
	return false
}

func gridFSIDToHex(id any) string {
	switch v := id.(type) {
	case bson.ObjectID:
		return v.Hex()
	case string:
		return v
	default:
		return ""
	}
}

func sanitizeImageContentType(declared, filename string, data []byte) string {
	lowerDeclared := strings.ToLower(strings.TrimSpace(strings.Split(declared, ";")[0]))
	if isAllowedImageMIME(lowerDeclared) {
		return lowerDeclared
	}

	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".svg":
		return "image/svg+xml"
	case ".tif", ".tiff":
		return "image/tiff"
	}

	if len(data) > 0 {
		detected := strings.ToLower(strings.Split(http.DetectContentType(data), ";")[0])
		if isAllowedImageMIME(detected) {
			return detected
		}
	}

	return "image/png"
}

func isAllowedImageMIME(mime string) bool {
	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp", "image/tiff", "image/svg+xml":
		return true
	default:
		return false
	}
}

func defaultExtensionForMIME(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/svg+xml":
		return ".svg"
	case "image/tiff":
		return ".tiff"
	default:
		return ".png"
	}
}