package main

import (
	"encoding/xml"
)

type BpmnDefinitions struct {
	XMLName     xml.Name    `xml:"bpmn:definitions"`
	XmlnsBpmn   string      `xml:"xmlns:bpmn,attr"`
	XmlnsBpmndi string      `xml:"xmlns:bpmndi,attr"`
	XmlnsDc     string      `xml:"xmlns:dc,attr"`
	XmlnsDi     string      `xml:"xmlns:di,attr"`
	ID          string      `xml:"id,attr"`
	TargetNS    string      `xml:"targetNamespace,attr"`
	Process     BpmnProcess `xml:"bpmn:process"`
	Diagram     BpmnDiagram `xml:"bpmndi:BPMNDiagram"`
}

type BpmnProcess struct {
	ID                string             `xml:"id,attr"`
	Name              string             `xml:"name,attr,omitempty"`
	IsExecutable      bool               `xml:"isExecutable,attr"`
	StartEvents       []BpmnNodeElement  `xml:"bpmn:startEvent"`
	EndEvents         []BpmnNodeElement  `xml:"bpmn:endEvent"`
	ServiceTasks      []BpmnNodeElement  `xml:"bpmn:serviceTask"`
	ExclusiveGateways []BpmnNodeElement  `xml:"bpmn:exclusiveGateway"`
	SequenceFlows     []BpmnSequenceFlow `xml:"bpmn:sequenceFlow"`
}

type BpmnNodeElement struct {
	ID   string `xml:"id,attr"`
	Name string `xml:"name,attr"`
}

type BpmnSequenceFlow struct {
	ID        string `xml:"id,attr"`
	Name      string `xml:"name,attr,omitempty"`
	SourceRef string `xml:"sourceRef,attr"`
	TargetRef string `xml:"targetRef,attr"`
}

type BpmnDiagram struct {
	ID    string    `xml:"id,attr"`
	Plane BpmnPlane `xml:"bpmndi:BPMNPlane"`
}

type BpmnPlane struct {
	ID          string          `xml:"id,attr"`
	BpmnElement string          `xml:"bpmnElement,attr"`
	Shapes      []BpmnShape     `xml:"bpmndi:BPMNShape"`
	Edges       []BpmnEdgeShape `xml:"bpmndi:BPMNEdge"`
}

type BpmnShape struct {
	ID          string     `xml:"id,attr"`
	BpmnElement string     `xml:"bpmnElement,attr"`
	Bounds      BpmnBounds `xml:"dc:Bounds"`
}

type BpmnBounds struct {
	X      int `xml:"x,attr"`
	Y      int `xml:"y,attr"`
	Width  int `xml:"width,attr"`
	Height int `xml:"height,attr"`
}

type BpmnEdgeShape struct {
	ID          string         `xml:"id,attr"`
	BpmnElement string         `xml:"bpmnElement,attr"`
	Waypoints   []BpmnWaypoint `xml:"di:waypoint"`
}

type BpmnWaypoint struct {
	X int `xml:"x,attr"`
	Y int `xml:"y,attr"`
}

func BuildBpmnXML(title string, nodes []Node, edges []Edge) ([]byte, error) {
	process := BpmnProcess{ID: "Process_SOC", Name: title, IsExecutable: true}
	plane := BpmnPlane{ID: "BPMNPlane_1", BpmnElement: process.ID}
	boundsMap := make(map[string]BpmnBounds)
	depths := calculateGraphDepths(nodes, edges)
	colsByDepth := make(map[int]int)

	inDegree := make(map[string]int, len(nodes))
	for _, e := range edges {
		inDegree[e.Target]++
	}

	for _, n := range nodes {
		d := depths[n.ID]
		col := colsByDepth[d]
		colsByDepth[d] = col + 1

		x, y, w, h := 160+(col*220), 80+(d*130), 120, 80
		if n.Type == "terminal" && inDegree[n.ID] == 0 {
			process.StartEvents = append(process.StartEvents, BpmnNodeElement{ID: n.ID, Name: n.Label})
			w, h = 36, 36
		} else if n.Type == "terminal" || n.Type == "end" {
			process.EndEvents = append(process.EndEvents, BpmnNodeElement{ID: n.ID, Name: n.Label})
			w, h = 36, 36
		} else if n.Type == "decision" {
			process.ExclusiveGateways = append(process.ExclusiveGateways, BpmnNodeElement{ID: n.ID, Name: n.Label})
			w, h = 50, 50
		} else {
			process.ServiceTasks = append(process.ServiceTasks, BpmnNodeElement{ID: n.ID, Name: n.Label})
		}

		b := BpmnBounds{X: x, Y: y, Width: w, Height: h}
		boundsMap[n.ID] = b
		plane.Shapes = append(plane.Shapes, BpmnShape{ID: n.ID + "_di", BpmnElement: n.ID, Bounds: b})
	}

	for _, e := range edges {
		process.SequenceFlows = append(process.SequenceFlows, BpmnSequenceFlow{
			ID: e.ID, Name: e.Label, SourceRef: e.Source, TargetRef: e.Target,
		})
		s, t := boundsMap[e.Source], boundsMap[e.Target]
		plane.Edges = append(plane.Edges, BpmnEdgeShape{
			ID: e.ID + "_di", BpmnElement: e.ID,
			Waypoints: []BpmnWaypoint{{X: s.X + s.Width/2, Y: s.Y + s.Height}, {X: t.X + t.Width/2, Y: t.Y}},
		})
	}

	defs := BpmnDefinitions{
		XmlnsBpmn:   "http://www.omg.org/spec/BPMN/20100524/MODEL",
		XmlnsBpmndi: "http://www.omg.org/spec/BPMN/20100524/DI",
		XmlnsDc:     "http://www.omg.org/spec/DD/20100524/DC",
		XmlnsDi:     "http://www.omg.org/spec/DD/20100524/DI",
		ID:          "Definitions_SOC",
		TargetNS:    "http://bpmn.io/schema/bpmn",
		Process:     process,
		Diagram:     BpmnDiagram{ID: "BPMNDiagram_1", Plane: plane},
	}

	out, err := xml.MarshalIndent(defs, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

func calculateGraphDepths(nodes []Node, edges []Edge) map[string]int {
	depths := make(map[string]int)
	if len(nodes) == 0 {
		return depths
	}
	depths[nodes[0].ID] = 0
	for range nodes {
		changed := false
		for _, e := range edges {
			if sd, ok := depths[e.Source]; ok {
				if td, exists := depths[e.Target]; !exists || sd+1 > td {
					depths[e.Target] = sd + 1
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}
	return depths
}