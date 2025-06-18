package mongo_driver

// Operator 操作集
type Operator D

// OperatorBuilder 操作集构造器
type OperatorBuilder struct {
	operators Operator
}

// NewOperatorBuilder 创建操作集构造器
func NewOperatorBuilder() *OperatorBuilder {
	return &OperatorBuilder{
		operators: Operator{},
	}
}

// Build 构建操作集
func (ob *OperatorBuilder) Build() Operator {
	return ob.operators
}

// Set 设置操作
func (ob *OperatorBuilder) Set(updates ...E) *OperatorBuilder {
	if len(updates) > 0 {
		ob.operators = append(ob.operators, E{Key: "$set", Value: updates})
	}
	return ob
}

// Unset 删除字段
func (ob *OperatorBuilder) Unset(updates ...E) *OperatorBuilder {
	if len(updates) > 0 {
		ob.operators = append(ob.operators, E{Key: "$unset", Value: updates})
	}
	return ob
}

// Increment 增加字段的值 支持负数以减少值
func (ob *OperatorBuilder) Increment(updates ...E) *OperatorBuilder {
	if len(updates) > 0 {
		ob.operators = append(ob.operators, E{Key: "$inc", Value: updates})
	}
	return ob
}

// Rename 重命名字段
func (ob *OperatorBuilder) Rename(updates ...E) *OperatorBuilder {
	if len(updates) > 0 {
		ob.operators = append(ob.operators, E{Key: "$rename", Value: updates})
	}
	return ob
}

// SetOnUpdate 在插入文档时设置字段值 仅在插入期间有效
func (ob *OperatorBuilder) SetOnUpdate(updates ...E) *OperatorBuilder {
	if len(updates) > 0 {
		ob.operators = append(ob.operators, E{Key: "$setOnInsert", Value: updates})
	}
	return ob
}

// Push 往数组添加元素
// example:
//
//		ob.Push(E{Key: "key", Value: "value"})
//	 添加多个元素需要使用 $each
//		ob.Push(E{Key: "key", Value: D{{Key: "$each", Value: []string{"value1", "value2"}}}})
func (ob *OperatorBuilder) Push(updates ...E) *OperatorBuilder {
	if len(updates) > 0 {
		ob.operators = append(ob.operators, E{Key: "$push", Value: updates})
	}
	return ob
}

// Pull 从数组中删除元素
func (ob *OperatorBuilder) Pull(updates ...E) *OperatorBuilder {
	if len(updates) > 0 {
		ob.operators = append(ob.operators, E{Key: "$pull", Value: updates})
	}
	return ob
}

// Pop 移除数组中的第一个或最后一个元素
// 1 移除第一个元素，-1 移除最后一个元素
// example:
// Pop(E{Key: "key", Value: 1})
func (ob *OperatorBuilder) Pop(update ...E) *OperatorBuilder {
	if len(update) > 0 {
		ob.operators = append(ob.operators, E{Key: "$pop", Value: update})
	}
	return ob
}

// PullAll 从数组中删除与指定值匹配的所有元素
func (ob *OperatorBuilder) PullAll(updates ...E) *OperatorBuilder {
	if len(updates) > 0 {
		ob.operators = append(ob.operators, E{Key: "$pullAll", Value: updates})
	}
	return ob
}

// AddToSet 向数组中添加唯一元素
// example:
//
//	ob.AddToSet(E{Key: "name", Value: "test"})
func (ob *OperatorBuilder) AddToSet(updates ...E) *OperatorBuilder {
	if len(updates) > 0 {
		ob.operators = append(ob.operators, E{Key: "$addToSet", Value: updates})
	}
	return ob
}

// CurrentDate 设置字段为当前日期
// example:
//
//	ob.CurrentDate(E{Key: "created_at", Value: true})
//	ob.CurrentDate(E{Key: "updated_at", Value: D{{Key: "$type", Value: "timestamp"}}})
func (ob *OperatorBuilder) CurrentDate(updates ...E) *OperatorBuilder {
	if len(updates) > 0 {
		ob.operators = append(ob.operators, E{Key: "$currentDate", Value: updates})
	}
	return ob
}

// Bit 按位运算 比如 and or xor
// example:
//
// ob.Bit(E{Key: "mask", Value: D{{Key: "and", Value: 1}, {Key: "or", Value: 2}}})
func (ob *OperatorBuilder) Bit(updates ...E) *OperatorBuilder {
	if len(updates) > 0 {
		ob.operators = append(ob.operators, E{Key: "$bit", Value: updates})
	}
	return ob
}

// ReplaceWith 替换整个文档的内容
func (ob *OperatorBuilder) ReplaceWith(replaceWith D) {
	ob.operators = append(ob.operators, E{Key: "$replaceWith", Value: replaceWith})
}
