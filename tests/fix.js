/**
 * 存储localStorage
 */
export const setStore = (name, content) => {
  if (!name) return
  if (typeof content !== 'string') {
    content = JSON.stringify(content)
  }
  window.localStorage.setItem(name, content)
}

/**
 * 获取localStorage
 */
export const getStore = name => {
  if (!name) return
  return window.localStorage.getItem(name)
}

/**
 * 删除localStorage
 */
export const removeStore = name => {
  if (!name) return
  window.localStorage.removeItem(name)
}

function addZero (i) {
  return i < 10 ? `0${i}` : i
}

export const getTime = (time, form) => {
  const date = new Date(time)
  const Y = date.getFullYear() + '-'
  const M = addZero(date.getMonth() + 1) + '-'
  const D = addZero(date.getDate()) + ' '
  const h = addZero(date.getHours()) + ':'
  const m = addZero(date.getMinutes()) + ':'
  const s = addZero(date.getSeconds())
  if(form) {
    return Y + M + D
  } else {
    return Y + M + D + h + m + s
  }
}
