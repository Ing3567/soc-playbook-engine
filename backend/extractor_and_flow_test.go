package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

var samplePNGBytes = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
	0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
	0x42, 0x60, 0x82,
}

var samplePNGBytes2 = append(append([]byte{}, samplePNGBytes...), 0x00)

func buildTestZip(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func buildSampleDocx(t *testing.T) []byte {
	t.Helper()
	relsXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://www.virustotal.com/gui/ip-address/1.1.1.1" TargetMode="External"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://splunk.corp.local/en-US/app/search/search?q=search%20index%3Dmain%20%7C%20stats%20count" TargetMode="External"/>
  <Relationship Id="rIdImg1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/vt_screen.png"/>
  <Relationship Id="rIdImg2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/fw_block.png"/>
</Relationships>`

	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
            xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
            xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
            xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
  <w:body>
    <w:p>
      <w:r><w:t>1. ตรวจสอบ IP ต้องสงสัยผ่าน </w:t></w:r>
      <w:hyperlink r:id="rId1"><w:r><w:t>VirusTotal Portal</w:t></w:r></w:hyperlink>
      <w:r>
        <w:drawing>
          <wp:inline>
            <wp:docPr id="1" name="Picture 1" descr="ผลตรวจ VirusTotal"/>
            <a:graphic><a:graphicData><pic><blipFill><a:blip r:embed="rIdImg1"/></blipFill></pic></a:graphicData></a:graphic>
          </wp:inline>
        </w:drawing>
      </w:r>
    </w:p>
    <w:p>
      <w:r><w:t>ถ้าพบว่าเป็นอันตราย</w:t></w:r>
    </w:p>
    <w:p>
      <w:pPr><w:numPr><w:ilvl w:val="1"/></w:numPr></w:pPr>
      <w:r><w:t>บล็อก IP บนระบบ </w:t></w:r>
      <w:r><w:fldChar w:fldCharType="begin"/></w:r>
      <w:r><w:instrText> HYPERLINK "https://fw.corp.local/block" </w:instrText></w:r>
      <w:r><w:fldChar w:fldCharType="separate"/></w:r>
      <w:r><w:t>Firewall Console</w:t></w:r>
      <w:r><w:fldChar w:fldCharType="end"/></w:r>
    </w:p>
    <w:p>
      <w:r>
        <w:drawing>
          <wp:inline>
            <wp:docPr id="2" name="Picture 2" descr="หน้าจอบล็อก Firewall"/>
            <a:graphic><a:graphicData><pic><blipFill><a:blip r:embed="rIdImg2"/></blipFill></pic></a:graphicData></a:graphic>
          </wp:inline>
        </w:drawing>
      </w:r>
    </w:p>
    <w:tbl>
      <w:tr>
        <w:tc><w:p><w:r><w:t>ลำดับ</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>ขั้นตอน</w:t></w:r></w:p></w:tc>
      </w:tr>
      <w:tr>
        <w:tc><w:p><w:r><w:t>2</w:t></w:r></w:p></w:tc>
        <w:tc>
          <w:p>
            <w:r><w:t>ค้นหาประวัติใน </w:t></w:r>
            <w:hyperlink r:id="rId2"><w:r><w:t>Splunk SIEM</w:t></w:r></w:hyperlink>
          </w:p>
        </w:tc>
      </w:tr>
    </w:tbl>
  </w:body>
</w:document>`

	return buildTestZip(t, map[string][]byte{
		"word/_rels/document.xml.rels": []byte(relsXML),
		"word/document.xml":            []byte(docXML),
		"word/media/vt_screen.png":     samplePNGBytes,
		"word/media/fw_block.png":      samplePNGBytes2,
	})
}

func buildSampleXlsx(t *testing.T) []byte {
	t.Helper()
	workbookXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"
          xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="SOP" sheetId="1" r:id="rIdSheet1"/>
  </sheets>
</workbook>`

	workbookRelsXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rIdSheet1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
</Relationships>`

	sharedStringsXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="6" uniqueCount="6">
  <si><t>ลำดับ</t></si>
  <si><t>ขั้นตอน</t></si>
  <si><t>ตรวจสอบ IP ต้องสงสัย</t></si>
  <si><t>VirusTotal Link</t></si>
  <si><t>ถ้าพบความผิดปกติ</t></si>
  <si><t>ค้นหา Log ใน SIEM</t></si>
</sst>`

	sheet1XML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"
           xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheetData>
    <row r="1">
      <c r="A1" t="s"><v>0</v></c>
      <c r="B1" t="s"><v>1</v></c>
    </row>
    <row r="2">
      <c r="A2" t="inlineStr"><is><t>1</t></is></c>
      <c r="B2" t="s"><v>2</v></c>
      <c r="C2" t="s"><v>3</v></c>
    </row>
    <row r="3">
      <c r="A3" t="inlineStr"><is><t>2</t></is></c>
      <c r="B3" t="s"><v>4</v></c>
    </row>
    <row r="4">
      <c r="A4" t="inlineStr"><is><t>2.1</t></is></c>
      <c r="B4" t="s"><v>5</v></c>
      <c r="C4" t="str">
        <f>HYPERLINK("https://splunk.corp.local/en-US/app/search/search?q=index%3Dauth%20action%3Dfailure","Splunk Auth Query")</f>
        <v>Splunk Auth Query</v>
      </c>
      <c r="D4" t="e" vm="1"><v>#VALUE!</v></c>
    </row>
  </sheetData>
  <hyperlinks>
    <hyperlink ref="C2" r:id="rIdLink1" display="VirusTotal Link"/>
  </hyperlinks>
  <drawing r:id="rIdDraw1"/>
</worksheet>`

	sheet1RelsXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rIdLink1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://www.virustotal.com/gui/ip-address/8.8.8.8" TargetMode="External"/>
  <Relationship Id="rIdDraw1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/drawing" Target="../drawings/drawing1.xml"/>
</Relationships>`

	drawing1XML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<xdr:wsDr xmlns:xdr="http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing"
          xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
          xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <xdr:twoCellAnchor>
    <xdr:from><xdr:col>3</xdr:col><xdr:row>1</xdr:row></xdr:from>
    <xdr:to><xdr:col>4</xdr:col><xdr:row>2</xdr:row></xdr:to>
    <xdr:pic>
      <xdr:nvPicPr><xdr:cNvPr id="1" name="VT Screenshot" descr="ภาพหน้าจอ VT จาก Excel"/></xdr:nvPicPr>
      <xdr:blipFill><a:blip r:embed="rIdImgExcel1"/></xdr:blipFill>
    </xdr:pic>
  </xdr:twoCellAnchor>
</xdr:wsDr>`

	drawing1RelsXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rIdImgExcel1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/excel_vt.png"/>
</Relationships>`

	richValueRelXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<richValueRels xmlns="http://schemas.microsoft.com/office/spreadsheetml/2022/richvaluerel"
               xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <rel r:id="rIdRich1"/>
</richValueRels>`

	richValueRelRelsXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rIdRich1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/excel_incell.png"/>
</Relationships>`

	return buildTestZip(t, map[string][]byte{
		"xl/workbook.xml":                           []byte(workbookXML),
		"xl/_rels/workbook.xml.rels":                []byte(workbookRelsXML),
		"xl/sharedStrings.xml":                      []byte(sharedStringsXML),
		"xl/worksheets/sheet1.xml":                  []byte(sheet1XML),
		"xl/worksheets/_rels/sheet1.xml.rels":       []byte(sheet1RelsXML),
		"xl/drawings/drawing1.xml":                  []byte(drawing1XML),
		"xl/drawings/_rels/drawing1.xml.rels":       []byte(drawing1RelsXML),
		"xl/richData/richValueRel.xml":              []byte(richValueRelXML),
		"xl/richData/_rels/richValueRel.xml.rels":   []byte(richValueRelRelsXML),
		"xl/media/excel_vt.png":                     samplePNGBytes,
		"xl/media/excel_incell.png":                 samplePNGBytes2,
	})
}

func TestDocxExtractionAndFlowchartEmbedding(t *testing.T) {
	Config.LlmProvider = "fallback"
	repo := NewInMemoryPlaybookRepository()
	docxBytes := buildSampleDocx(t)

	extracted, err := ExtractLinesFromDocx(docxBytes)
	if err != nil {
		t.Fatalf("ExtractLinesFromDocx failed: %v", err)
	}
	if len(extracted) != 4 {
		t.Fatalf("expected 4 extracted lines (with standalone image merged and table header skipped), got %d: %+v", len(extracted), extracted)
	}

	// Line 0: step 1 with link + inline image
	if len(extracted[0].Links) != 1 || extracted[0].Links[0].Label != "VirusTotal Portal" {
		t.Errorf("unexpected line 0 links: %+v", extracted[0].Links)
	}
	if len(extracted[0].Images) != 1 || extracted[0].Images[0].Caption != "ผลตรวจ VirusTotal" {
		t.Errorf("unexpected line 0 images: %+v", extracted[0].Images)
	}

	// Line 2: indented firewall step with field-code hyperlink + merged standalone image from paragraph 4
	if extracted[2].Level != 1 {
		t.Errorf("expected line 2 level 1, got %d", extracted[2].Level)
	}
	if len(extracted[2].Links) != 1 || extracted[2].Links[0].URL != "https://fw.corp.local/block" {
		t.Errorf("unexpected line 2 links: %+v", extracted[2].Links)
	}
	if len(extracted[2].Images) != 1 || extracted[2].Images[0].Caption != "หน้าจอบล็อก Firewall" {
		t.Errorf("expected standalone image to merge into line 2, got: %+v", extracted[2].Images)
	}

	// Line 3: table row with Splunk hyperlink
	if len(extracted[3].Links) != 1 || !strings.Contains(extracted[3].Links[0].URL, "splunk.corp.local") {
		t.Errorf("unexpected line 3 table links: %+v", extracted[3].Links)
	}

	ctx := context.Background()
	displayLines, maskedLines, registry := PrepareLinesAndRegistry(ctx, extracted, repo)
	steps, _ := ConvertLinesToSteps(maskedLines)
	nodes, _ := BuildCustomFlowchartWithRegistry(steps, registry)

	var foundVTNode, foundFWNode, foundSplunkNode bool
	for _, n := range nodes {
		if strings.Contains(n.Label, "[[URL_") || strings.Contains(n.Label, "[[IMG_") {
			t.Errorf("node label still contains placeholder token: %s", n.Label)
		}
		if strings.Contains(n.Label, "VirusTotal") {
			foundVTNode = true
			if len(n.Links) != 1 || n.Links[0].Label != "VirusTotal Portal" {
				t.Errorf("VT node missing link: %+v", n.Links)
			}
			if len(n.Images) != 1 || !strings.HasPrefix(n.Images[0].URL, "/api/images/") {
				t.Errorf("VT node missing saved MongoDB image: %+v", n.Images)
			}
		}
		if strings.Contains(n.Label, "Firewall") {
			foundFWNode = true
			if len(n.Links) != 1 || n.Links[0].URL != "https://fw.corp.local/block" {
				t.Errorf("Firewall node missing link: %+v", n.Links)
			}
			if len(n.Images) != 1 || n.Images[0].Caption != "หน้าจอบล็อก Firewall" {
				t.Errorf("Firewall node missing image: %+v", n.Images)
			}
		}
		if strings.Contains(n.Label, "Splunk") {
			foundSplunkNode = true
			if len(n.Links) != 1 || n.Links[0].Source != "splunk" || n.Links[0].SplunkQuery != "search index=main | stats count" {
				t.Errorf("Splunk node missing parsed splunk query link: %+v", n.Links)
			}
		}
	}

	if !foundVTNode || !foundFWNode || !foundSplunkNode {
		t.Fatalf("missing expected nodes (VT=%v, FW=%v, Splunk=%v) in %+v", foundVTNode, foundFWNode, foundSplunkNode, nodes)
	}

	// Verify re-generation from displayLines text (simulating user editing textarea and clicking Generate Flowchart)
	var rawLines []string
	for _, dl := range displayLines {
		rawLines = append(rawLines, strings.Repeat("  ", dl.Level)+dl.Text)
	}
	reExtracted := ExtractLinesFromText(strings.Join(rawLines, "\n"))
	_, reMasked, reRegistry := PrepareLinesAndRegistry(ctx, reExtracted, repo)
	reSteps, _ := ConvertLinesToSteps(reMasked)
	reNodes, _ := BuildCustomFlowchartWithRegistry(reSteps, reRegistry)

	totalReLinks := 0
	totalReImages := 0
	for _, n := range reNodes {
		totalReLinks += len(n.Links)
		totalReImages += len(n.Images)
	}
	if totalReLinks != 3 || totalReImages != 2 {
		t.Errorf("expected 3 links and 2 images after re-generating from textarea, got %d links and %d images", totalReLinks, totalReImages)
	}
}

func TestXlsxExtractionAndHttpHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	Config.LlmProvider = "fallback"
	repo := NewInMemoryPlaybookRepository()
	handler := NewApiHandler(repo)
	router := gin.New()
	handler.RegisterRoutes(router)

	xlsxBytes := buildSampleXlsx(t)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "soc_incident_playbook.xlsx")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(xlsxBytes); err != nil {
		t.Fatalf("write xlsx part: %v", err)
	}
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/generate", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/generate with .xlsx returned %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Nodes  []Node `json:"nodes"`
		Edges  []Edge `json:"edges"`
		Engine string `json:"engine"`
		Lines  []Line `json:"lines"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	var imageURLs []string
	var foundVT, foundSplunk bool
	for _, n := range resp.Nodes {
		for _, img := range n.Images {
			imageURLs = append(imageURLs, img.URL)
		}
		for _, l := range n.Links {
			if l.URL == "https://www.virustotal.com/gui/ip-address/8.8.8.8" {
				foundVT = true
			}
			if l.Source == "splunk" && l.SplunkQuery == "index=auth action=failure" {
				foundSplunk = true
			}
		}
	}

	if !foundVT {
		t.Errorf("expected VirusTotal hyperlink from Excel worksheet <hyperlinks>, nodes=%+v", resp.Nodes)
	}
	if !foundSplunk {
		t.Errorf("expected Splunk link from Excel HYPERLINK() formula, nodes=%+v", resp.Nodes)
	}
	if len(imageURLs) != 2 {
		t.Fatalf("expected 2 images (1 floating drawing + 1 in-cell richData image), got %d: %v", len(imageURLs), imageURLs)
	}

	// Verify GET /api/images/:id serves the stored image binary from repository
	for _, imgURL := range imageURLs {
		imgReq := httptest.NewRequest(http.MethodGet, imgURL, nil)
		imgRec := httptest.NewRecorder()
		router.ServeHTTP(imgRec, imgReq)
		if imgRec.Code != http.StatusOK {
			t.Errorf("GET %s returned %d", imgURL, imgRec.Code)
		}
		if imgRec.Header().Get("Content-Type") != "image/png" {
			t.Errorf("expected Content-Type image/png, got %s", imgRec.Header().Get("Content-Type"))
		}
		if !bytes.HasPrefix(imgRec.Body.Bytes(), []byte{0x89, 0x50, 0x4E, 0x47}) {
			t.Errorf("GET %s body is not valid PNG bytes", imgURL)
		}
	}

	// Save playbook and verify DELETE cleans up unreferenced images
	savePayload, _ := json.Marshal(PlaybookDocument{
		Title: "Excel SOP",
		Nodes: resp.Nodes,
		Edges: resp.Edges,
	})
	saveReq := httptest.NewRequest(http.MethodPost, "/api/sops", bytes.NewReader(savePayload))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	router.ServeHTTP(saveRec, saveReq)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("POST /api/sops returned %d", saveRec.Code)
	}
	var savedDoc PlaybookDocument
	_ = json.Unmarshal(saveRec.Body.Bytes(), &savedDoc)

	delReq := httptest.NewRequest(http.MethodDelete, "/api/sops/"+savedDoc.ID, nil)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("DELETE /api/sops/%s returned %d", savedDoc.ID, delRec.Code)
	}

	// Image should now be cleaned up (404)
	imgReqAfter := httptest.NewRequest(http.MethodGet, imageURLs[0], nil)
	imgRecAfter := httptest.NewRecorder()
	router.ServeHTTP(imgRecAfter, imgReqAfter)
	if imgRecAfter.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unreferenced image after playbook delete, got %d", imgRecAfter.Code)
	}
}

func TestLLMTokenReconciliation(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryPlaybookRepository()
	extracted := []ExtractedLine{
		{
			Level: 0,
			Text:  "ตรวจสอบ IP ต้องสงสัยผ่าน VirusTotal",
			Links: []ExtractedLink{{Label: "VT Link", URL: "https://www.virustotal.com/gui/ip-address/9.9.9.9"}},
			Images: []ExtractedImage{{
				Filename:    "vt.png",
				ContentType: "image/png",
				Data:        samplePNGBytes,
				Caption:     "VT Screenshot",
			}},
		},
	}

	_, _, registry := PrepareLinesAndRegistry(ctx, extracted, repo)

	// Simulate an LLM response that dropped [[URL_1]] and [[IMG_1]] from the step JSON
	llmSteps := []Step{
		{
			Type:  "lookup",
			Label: "ตรวจสอบ IP ต้องสงสัยผ่าน VirusTotal",
		},
	}

	nodes, _ := BuildCustomFlowchartWithRegistry(llmSteps, registry)
	var lookupNode *Node
	for i := range nodes {
		if nodes[i].Type == "lookup" {
			lookupNode = &nodes[i]
			break
		}
	}
	if lookupNode == nil {
		t.Fatalf("lookup node not found in %+v", nodes)
	}
	if len(lookupNode.Links) != 1 || lookupNode.Links[0].URL != "https://www.virustotal.com/gui/ip-address/9.9.9.9" {
		t.Errorf("expected ReconcileSteps to recover dropped link onto lookup node, got %+v", lookupNode.Links)
	}
	if len(lookupNode.Images) != 1 || lookupNode.Images[0].Caption != "VT Screenshot" {
		t.Errorf("expected ReconcileSteps to recover dropped image onto lookup node, got %+v", lookupNode.Images)
	}
}

func TestDocxNestedTextboxParagraphPreservesOuterAssets(t *testing.T) {
	relsXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rIdImg1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/outer.png"/>
</Relationships>`

	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
            xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
            xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
  <w:body>
    <w:p>
      <w:r><w:t>ตรวจสอบข้อมูลหลัก</w:t></w:r>
      <w:r>
        <w:drawing>
          <a:blip r:embed="rIdImg1"/>
          <w:txbxContent><w:p><w:r><w:t>ข้อความในกล่อง</w:t></w:r></w:p></w:txbxContent>
        </w:drawing>
      </w:r>
    </w:p>
  </w:body>
</w:document>`

	docxBytes := buildTestZip(t, map[string][]byte{
		"word/_rels/document.xml.rels": []byte(relsXML),
		"word/document.xml":            []byte(docXML),
		"word/media/outer.png":         samplePNGBytes,
	})

	lines, err := ExtractLinesFromDocx(docxBytes)
	if err != nil {
		t.Fatalf("ExtractLinesFromDocx failed: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %+v", len(lines), lines)
	}
	if !strings.Contains(lines[0].Text, "ตรวจสอบข้อมูลหลัก") {
		t.Errorf("outer paragraph text lost: %q", lines[0].Text)
	}
	if len(lines[0].Images) != 1 {
		t.Errorf("outer paragraph image lost: %+v", lines[0].Images)
	}
}

func TestResolveNodeLinksIgnoresMarkdownImageURLs(t *testing.T) {
	reg := NewURLRegistry()
	links := reg.ResolveNodeLinks(nil, "ตรวจสอบรูป ![chart](https://cdn.example.com/chart.png) และ [Portal](https://portal.example.com)")
	if len(links) != 1 || links[0].URL != "https://portal.example.com" {
		t.Fatalf("expected only portal link without image URL, got %+v", links)
	}
}

func TestLegacyOfficeFormatsRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewApiHandler(NewInMemoryPlaybookRepository())
	router := gin.New()
	handler.RegisterRoutes(router)

	for _, fname := range []string{"old.doc", "old.xls"} {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		part, _ := mw.CreateFormFile("file", fname)
		_, _ = part.Write([]byte("legacy binary"))
		_ = mw.Close()

		req := httptest.NewRequest(http.MethodPost, "/api/generate", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for %s, got %d", fname, rec.Code)
		}
	}
}

