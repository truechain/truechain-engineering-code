package core

import "sort"


// 把数组看成是一颗满的二叉树，第一个元素是树根，第二和第三个元素是树根的两个树枝，
// 这样依次推下去 那么如果树根是  i 那么它的两个树枝就是 2*i+2 和 2*i + 2。
// 最小堆的定义是 任意的树根不能比它的两个树枝大。 也就是上面的代码描述的定义。
//heap.Interface的定义


type Interface interface {
	sort.Interface
	Push(x interface{}) // add x as element Len() 把x增加到最后
	Pop() interface{}   //  remove and return element Len() - 1. 移除并返回最后的一个元素
}

// nonceHeap is a heap.Interface implementation over 64bit unsigned integers for
// retrieving sorted transactions from the possibly gapped future queue.
type nonceHeap []uint64

func (h nonceHeap) Len() int           { return len(h) }
func (h nonceHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h nonceHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *nonceHeap) Push(x interface{}) {
	*h = append(*h, x.(uint64))
}

func (h *nonceHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
