/**
 * dorpDownList.js 1.0
 * @version 1.0
 * @author zhuzhida
 * @url https://github.com/richard1015/dropDownList
 *
 * @调用方法
 * $(selector).dropdownlist(option);
 * option里的callback是选择国家后调用
 *
 * -- example --
 * $(selector).dropdownlist({
 *     defaultText: "请选择国家", //默认提示语
 *     selectIds: "",//默认选中国家
 *     callback: function(api) {
 *         console.log('选择下拉选项调用该回调');
 *     }
 * });
 */
(function (factory) {
    if (typeof define === "function" && (define.amd || define.cmd) && !jQuery) {
        // AMD或CMD
        define(["jquery"], factory);
    } else if (typeof module === 'object' && module.exports) {
        // Node/CommonJS
        module.exports = function (root, jQuery) {
            if (jQuery === undefined) {
                if (typeof window !== 'undefined') {
                    jQuery = require('jquery');
                } else {
                    jQuery = require('jquery')(root);
                }
            }
            factory(jQuery);
            return jQuery;
        };
    } else {
        //Browser globals
        factory(jQuery);
    }
}(function ($) {
    //配置参数
    var defaults = {
        data: [{
            id: 12,
            name: '澳洲',
            isChecked: false
        },
        {
            id: 10,
            name: '美国',
            isChecked: false
        },
        {
            id: 20,
            name: '英国',
            isChecked: false
        },
        {
            id: 11,
            name: '加拿大',
            isChecked: false
        },
        {
            id: 13,
            name: '新西兰',
            isChecked: false
        }], //默认数据
        defaultText: "请选择国家",
        selectIds: "",//默认选择国家
        callback: function () { } //回调
    };

    var DropDownList = function (element, options) {
        //全局变量
        var opts = options, //配置
            resultArray = [], //当前数组
            $document = $(document),
            $obj = $(element); //容器

        // 检查默认选中项
        if (opts.selectIds) {
            var ids = opts.selectIds.split(",");
            ids.forEach(id => {
                var index = opts.data.findIndex(item => item.id == id);
                opts.data[index].isChecked = true;
            });
        }
        /**
         * 填充数据
         */
        this.filling = function () {
            var ulStr = `<div class="dropDownList"><button>${opts.defaultText}<i class="iconfont icon-paixu_xiangxia"></i></button><ul>`,
                ulBody = "";
            opts.data.forEach(element => {
                if (element.isChecked) {
                    resultArray.push(element)
                }
                ulBody +=
                    `<li data-id="${element.id}" data-name="${element.name}">
                    <label class="ivu-checkbox-wrapper ivu-checkbox-group-item ${element.isChecked == true ? "ivu-checkbox-wrapper-checked" : ""}">
                    <span class="ivu-checkbox ${element.isChecked == true ? "ivu-checkbox-checked" : ""}"">
                        <span class="ivu-checkbox-inner"></span>
                        <input type="checkbox" class="ivu-checkbox-input"/>
                    </span>
                    <span>${element.name}</span>
                    </label>
                </li>`
            });
            ulBody += `</ul></div>`;
            if (resultArray.length) {
                var text = resultArray.map(item => { return item.name }).join("、");
                ulStr = `<div class="dropDownList"><button>${text}<i class="iconfont icon-paixu_xiangxia"></i></button><ul>`
            }
            $obj.empty().html(ulStr + ulBody);
            typeof opts.callback === 'function' && opts.callback(resultArray);
        };

        //绑定事件
        this.eventBind = function () {
            // 下拉列表框点击事件
            $obj.off().on('click', 'button', function (e) {
                e = e || window.event;
                if (e.stopPropagation) { //W3C阻止冒泡方法
                    e.stopPropagation();
                } else {
                    e.cancelBubble = true; //IE阻止冒泡方法
                }
                if ($obj.find("ul").css('display') == 'none') {
                    $obj.find("ul").show(300);
                    $obj.find("i").attr("class", "iconfont icon-paixu_xiangshang");
                } else {
                    $obj.find("ul").hide(300);
                    $obj.find("i").attr("class", "iconfont icon-paixu_xiangxia");
                }
            });
            // 下拉列表框点击事件
            $obj.on('click', 'li', function (e) {
                e = e || window.event;
                // js阻止链接默认行为，没有停止冒泡
                e.preventDefault();
                if (e.stopPropagation) { //W3C阻止冒泡方法
                    e.stopPropagation();
                } else {
                    e.cancelBubble = true; //IE阻止冒泡方法
                }
                var data = e.currentTarget.dataset;
                // console.log(data)
                // 取消选中
                if (e.currentTarget.children[0].className == "ivu-checkbox-wrapper ivu-checkbox-group-item ivu-checkbox-wrapper-checked") {
                    $(e.currentTarget.children[0]).attr("class", "ivu-checkbox-wrapper ivu-checkbox-group-item");
                    $(e.currentTarget.children[0].children[0]).attr("class", "ivu-checkbox");
                    var index = resultArray.findIndex(item => item.id == data.id);
                    resultArray.splice(index, 1);
                } else {//确定选中
                    $(e.currentTarget.children[0]).attr("class", "ivu-checkbox-wrapper ivu-checkbox-group-item ivu-checkbox-wrapper-checked");
                    $(e.currentTarget.children[0].children[0]).attr("class", "ivu-checkbox ivu-checkbox-checked");
                    resultArray.push(data);
                }
                // console.log(resultArray);
                if (resultArray.length) {
                    var text = resultArray.map(item => { return item.name }).join("、");
                    $obj.find("button").html(text + "<i class=\"iconfont icon-paixu_xiangshang\"></i>");
                } else {
                    $obj.find("button").html(opts.defaultText + "<i class=\"iconfont icon-paixu_xiangshang\"></i>");
                }

                typeof opts.callback === 'function' && opts.callback(resultArray);
            });
            //在document上监听click函数
            $(document).bind('click', function (event) {
                $obj.find("ul").hide(300);
                $obj.find("i").attr("class", "iconfont icon-paixu_xiangxia");
            });
        };

        //初始化
        this.init = function () {
            this.filling();
            this.eventBind();
        };
        this.init();
    };

    $.fn.dropdownlist = function (parameter, callback) {
        if (typeof parameter == 'function') { //重载
            callback = parameter;
            parameter = {};
        } else {
            parameter = parameter || {};
            callback = callback || function () { };
        }
        var options = $.extend({}, defaults, parameter);
        return this.each(function () {
            var dropdownlist = new DropDownList(this, options);
            callback(dropdownlist);
        });
    };
}));
