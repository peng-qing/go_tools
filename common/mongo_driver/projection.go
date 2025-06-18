package mongo_driver

// Projection 投影 用于控制文档返回字段的内容
type Projection D

// ProjectionBuilder 投影构建器
type ProjectionBuilder struct {
	fields      map[string]struct{}
	projections Projection
}

// NewProjectionBuilder 创建投影构建器
func NewProjectionBuilder() *ProjectionBuilder {
	return &ProjectionBuilder{
		projections: Projection{},
	}
}

// Build 构建投影
func (pb *ProjectionBuilder) Build() Projection {
	return pb.projections
}

// Fields 指定返回哪些字段 重复设置无效
func (pb *ProjectionBuilder) Fields(fields ...string) *ProjectionBuilder {
	if len(fields) <= 0 {
		return pb
	}
	for _, field := range fields {
		if _, ok := pb.fields[field]; ok {
			continue
		}
		pb.projections = append(pb.projections, E{Key: field, Value: 1})
	}
	return pb
}

// Excludes 指定不返回哪些字段
func (pb *ProjectionBuilder) Excludes(fields ...string) *ProjectionBuilder {
	if len(fields) <= 0 {
		return pb
	}
	for _, field := range fields {
		if _, ok := pb.fields[field]; ok {
			continue
		}
		pb.projections = append(pb.projections, E{Key: field, Value: 0})
	}
	return pb
}

// Only 指定返回单个字段 会排除 '_id'
func (pb *ProjectionBuilder) Only(field string) *ProjectionBuilder {
	pb.Fields(field).Excludes("_id")
	return pb
}

// FirstMatchSliceElem 仅返回数组中第一个匹配的元素
func (pb *ProjectionBuilder) FirstMatchSliceElem(field string) *ProjectionBuilder {
	pb.Only(field + ".$")
	return pb
}

// ElementMatch 仅返回数组中匹配的元素
func (pb *ProjectionBuilder) ElementMatch(field string, filter Filter) *ProjectionBuilder {
	if _, ok := pb.fields[field]; ok {
		return pb
	}
	pb.fields[field] = struct{}{}
	pb.projections = append(pb.projections, E{Key: field, Value: filter})
	return pb
}

// Slice 返回数组指定位置开始的指定个数元素
func (pb *ProjectionBuilder) Slice(field string, offset int, count int) *ProjectionBuilder {
	filter := NewFilterBuilder().Slice(field, offset, count).Build()
	return pb.ElementMatch(field, filter)
}
