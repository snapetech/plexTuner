package plex

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type EPGChannel struct {
	GuideNumber string
	GuideName   string
	TagValue    string
}

func SyncEPGToPlex(plexDataDir string, dvrUUID string, channels []EPGChannel) error {
	if len(channels) == 0 {
		return nil
	}

	dbPath := filepath.Join(plexDataDir, "Plug-in Support", "Databases", fmt.Sprintf("tv.plex.providers.epg.xmltv-%s.db", dvrUUID))
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("EPG database not found: %s", dbPath)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open EPG DB: %w", err)
	}
	defer db.Close()

	nowTs := int(time.Now().Unix())

	libCols, metaCols, tagCols, err := getEPGSchema(db)
	if err != nil {
		return fmt.Errorf("get EPG schema: %w", err)
	}

	existingMeta := 0
	db.QueryRow("SELECT COUNT(*) FROM metadata_items").Scan(&existingMeta)
	existingLib := 0
	db.QueryRow("SELECT COUNT(*) FROM library_sections").Scan(&existingLib)

	fmt.Printf("EPG sync: existing library_sections=%d, metadata_items=%d, channels=%d\n", existingLib, existingMeta, len(channels))

	sectionID := 1
	if existingLib > 0 {
		var id int
		if err := db.QueryRow("SELECT id FROM library_sections LIMIT 1").Scan(&id); err == nil {
			sectionID = id
		}
	} else {
		libRow := map[string]interface{}{
			"id":           1,
			"name":         "PlexTuner",
			"section_type": 8,
			"language":     "en",
			"agent":        fmt.Sprintf("tv.plex.providers.epg.xmltv:{\"key\": \"%s\"}", dvrUUID),
			"scanner":      "Plex DVR",
			"uuid":         dvrUUID,
			"created_at":   nowTs,
			"updated_at":   nowTs,
			"scanned_at":   nowTs,
		}
		validCols := filterCols(libCols, libRow)
		if len(validCols) > 0 {
			insertRow(db, "library_sections", validCols, libRow)
		}
	}

	channelTags, err := getChannelTags(db, tagCols)
	if err != nil {
		return fmt.Errorf("get channel tags: %w", err)
	}

	if len(channelTags) == 0 {
		return fmt.Errorf("no channel tags found in EPG DB - XMLTV may not have been loaded yet")
	}

	tagToMetaID := make(map[int]int)

	for _, ch := range channels {
		for _, tag := range channelTags {
			tagNum := extractChannelNumber(tag.Value)
			if tagNum == ch.GuideNumber {
				metaID, err := insertMetadataItem(db, metaCols, sectionID, ch, nowTs)
				if err != nil {
					continue
				}
				tagToMetaID[tag.ID] = metaID
				break
			}
		}
	}

	for tagID, metaID := range tagToMetaID {
		db.Exec("UPDATE tags SET metadata_item_id = ? WHERE id = ?", metaID, tagID)
	}

	return nil
}

func getEPGSchema(db *sql.DB) ([]string, []string, []string, error) {
	var libCols, metaCols, tagCols []string

	rows, _ := db.Query("PRAGMA table_info(library_sections)")
	if rows != nil {
		for rows.Next() {
			var id int
			var name, colType string
			rows.Scan(&id, &name, &colType, new([]byte), new([]byte), new([]byte))
			libCols = append(libCols, name)
		}
		rows.Close()
	}

	rows, _ = db.Query("PRAGMA table_info(metadata_items)")
	if rows != nil {
		for rows.Next() {
			var id int
			var name, colType string
			rows.Scan(&id, &name, &colType, new([]byte), new([]byte), new([]byte))
			metaCols = append(metaCols, name)
		}
		rows.Close()
	}

	rows, _ = db.Query("PRAGMA table_info(tags)")
	if rows != nil {
		for rows.Next() {
			var id int
			var name, colType string
			rows.Scan(&id, &name, &colType, new([]byte), new([]byte), new([]byte))
			tagCols = append(tagCols, name)
		}
		rows.Close()
	}

	return libCols, metaCols, tagCols, nil
}

type Tag struct {
	ID    int
	Value string
}

func getChannelTags(db *sql.DB, tagCols []string) ([]Tag, error) {
	hasMetadataItemID := false
	for _, c := range tagCols {
		if c == "metadata_item_id" {
			hasMetadataItemID = true
			break
		}
	}

	var rows *sql.Rows
	var err error
	if hasMetadataItemID {
		rows, err = db.Query("SELECT id, value FROM tags WHERE tag_type = 310 AND metadata_item_id IS NULL")
	} else {
		rows, err = db.Query("SELECT id, value FROM tags WHERE tag_type = 310")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		rows.Scan(&t.ID, &t.Value)
		tags = append(tags, t)
	}
	return tags, nil
}

func insertMetadataItem(db *sql.DB, metaCols []string, sectionID int, ch EPGChannel, nowTs int) (int, error) {
	chanNum := ch.GuideNumber
	if idx, err := strconv.Atoi(chanNum); err == nil {
		chanNum = fmt.Sprintf("%d", idx)
	}

	guid := fmt.Sprintf("xmltv://%s", ch.GuideNumber)
	if ch.GuideNumber == "" {
		guid = fmt.Sprintf("xmltv://%s", ch.GuideName)
	}

	metaRow := map[string]interface{}{
		"library_section_id": sectionID,
		"metadata_type":      1,
		"guid":               guid,
		"title":              ch.GuideName,
		"title_sort":         ch.GuideName,
		"index":              sectionID,
		"added_at":           nowTs,
		"created_at":         nowTs,
		"updated_at":         nowTs,
	}

	validCols := filterCols(metaCols, metaRow)
	if len(validCols) == 0 {
		return 0, fmt.Errorf("no valid columns for metadata_items")
	}

	placeholders := make([]string, len(validCols))
	args := make([]interface{}, len(validCols))
	for i, col := range validCols {
		placeholders[i] = "?"
		args[i] = metaRow[col]
	}

	sql := fmt.Sprintf("INSERT INTO metadata_items (%s) VALUES (%s)", strings.Join(validCols, ", "), strings.Join(placeholders, ", "))
	_, err := db.Exec(sql, args...)
	if err != nil {
		return 0, err
	}

	var id int
	db.QueryRow("SELECT last_insert_rowid()").Scan(&id)
	return id, nil
}

func filterCols(cols []string, row map[string]interface{}) []string {
	var valid []string
	for _, c := range cols {
		if _, ok := row[c]; ok {
			valid = append(valid, c)
		}
	}
	return valid
}

func insertRow(db *sql.DB, table string, cols []string, row map[string]interface{}) {
	placeholders := make([]string, len(cols))
	args := make([]interface{}, len(cols))
	for i, col := range cols {
		placeholders[i] = "?"
		args[i] = row[col]
	}
	sql := fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) VALUES (%s)", table, strings.Join(cols, ", "), strings.Join(placeholders, ", "))
	db.Exec(sql, args...)
}

func extractChannelNumber(tagValue string) string {
	if strings.Contains(tagValue, ",") {
		parts := strings.SplitN(tagValue, ",", 2)
		if len(parts) > 0 {
			nums := strings.Fields(parts[0])
			if len(nums) > 0 {
				return nums[len(nums)-1]
			}
		}
	}
	return strings.TrimSpace(tagValue)
}
