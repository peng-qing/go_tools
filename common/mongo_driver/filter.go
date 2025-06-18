package mongo_driver

// Filter 过滤器
type Filter D

// FilterBuilder 过滤器构建器
type FilterBuilder struct {
	fields map[string]Filter
}

// NewFilterBuilder 创建一个过滤器构建器
func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{
		fields: make(map[string]Filter),
	}
}

// Build 构建过滤器
func (fb *FilterBuilder) Build() Filter {
	filter := Filter{}
	for fieldName, fieldConditions := range fb.fields {
		filter = append(filter, E{Key: fieldName, Value: fieldConditions})
	}
	return filter
}

// AddOperator 添加操作
func (fb *FilterBuilder) addOperator(key string, operator string, value any) *FilterBuilder {
	fieldConditions, ok := fb.fields[key]
	if !ok {
		fieldConditions = Filter{}
	}

	fieldConditions = append(fieldConditions, E{
		Key:   operator,
		Value: value,
	})

	fb.fields[key] = fieldConditions

	return fb
}

// EQ 等于
func (fb *FilterBuilder) EQ(key string, value any) *FilterBuilder {
	return fb.addOperator(key, "$eq", value)
}

// NE 不等于
func (fb *FilterBuilder) NE(key string, value any) *FilterBuilder {
	return fb.addOperator(key, "$ne", value)
}

// GT 大于
func (fb *FilterBuilder) GT(key string, value any) *FilterBuilder {
	return fb.addOperator(key, "$gt", value)
}

// GTE 大于等于
func (fb *FilterBuilder) GTE(key string, value any) *FilterBuilder {
	return fb.addOperator(key, "$gte", value)
}

// LT 小于
func (fb *FilterBuilder) LT(key string, value any) *FilterBuilder {
	return fb.addOperator(key, "$lt", value)
}

// LTE 小于等于
func (fb *FilterBuilder) LTE(key string, value any) *FilterBuilder {
	return fb.addOperator(key, "$lte", value)
}

// CloseInterval 区间 左闭右闭
func (fb *FilterBuilder) CloseInterval(key string, min, max any) *FilterBuilder {
	return fb.addOperator(key, "$gte", min).
		addOperator(key, "$lte", max)
}

// LeftCloseRightOpen 区间 左闭右开
func (fb *FilterBuilder) LeftCloseRightOpen(key string, min, max any) *FilterBuilder {
	return fb.addOperator(key, "$gte", min).
		addOperator(key, "$lt", max)
}

// LeftOpenRightOpen 区间 左开右开
func (fb *FilterBuilder) LeftOpenRightOpen(key string, min, max any) *FilterBuilder {
	return fb.addOperator(key, "$gt", min).
		addOperator(key, "$lt", max)
}

// LeftOpenRightClose 区间 左开右闭
func (fb *FilterBuilder) LeftOpenRightClose(key string, min, max any) *FilterBuilder {
	return fb.addOperator(key, "$gt", min).addOperator(key, "$lte", max)
}

// IN 匹配数组元素 包含
func (fb *FilterBuilder) IN(key string, value any) *FilterBuilder {
	return fb.addOperator(key, "$in", value)
}

// NIN 匹配数组元素 不包含
func (fb *FilterBuilder) NIN(key string, value any) *FilterBuilder {
	return fb.addOperator(key, "$nin", value)
}

// ALL 匹配数组元素 全包含
func (fb *FilterBuilder) ALL(key string, value any) *FilterBuilder {
	return fb.addOperator(key, "$all", value)
}

// SIZE 匹配数组元素 长度
func (fb *FilterBuilder) SIZE(key string, value any) *FilterBuilder {
	return fb.addOperator(key, "$size", value)
}

// ElemMatch 匹配数组中某个元素 条件匹配
func (fb *FilterBuilder) ElemMatch(key string, filters Filter) *FilterBuilder {
	return fb.addOperator(key, "$elemMatch", filters)
}

// Slice 匹配数组中某个元素 索引范围
// offset: 索引起始位置 0开始 负数表示从末尾开始往前
// count: 元素数量
func (fb *FilterBuilder) Slice(key string, offset int, count int) *FilterBuilder {
	return fb.addOperator(key, "$slice", []int{offset, count})
}

// EXISTS 匹配字段是否存在
func (fb *FilterBuilder) EXISTS(key string, exists bool) *FilterBuilder {
	return fb.addOperator(key, "$exists", exists)
}

// TYPE 匹配字段类型
func (fb *FilterBuilder) TYPE(key string, exists bool) *FilterBuilder {
	return fb.addOperator(key, "$type", exists)
}

// AND 与 都满足
func (fb *FilterBuilder) AND(filters ...E) *FilterBuilder {
	return fb.addOperator("$and", "$and", filters)
}

// OR 或 任意满足
func (fb *FilterBuilder) OR(filters ...E) *FilterBuilder {
	if len(filters) <= 0 {
		return fb
	}
	return fb.addOperator("$or", "$or", filters)
}

// NOT 非 否定查询
func (fb *FilterBuilder) NOT(filters ...E) *FilterBuilder {
	if len(filters) <= 0 {
		return fb
	}
	return fb.addOperator("$not", "$not", filters)
}

// NOR 都不满足
func (fb *FilterBuilder) NOR(filters ...Filter) *FilterBuilder {
	if len(filters) <= 0 {
		return fb
	}
	return fb.addOperator("$nor", "$nor", filters)
}
