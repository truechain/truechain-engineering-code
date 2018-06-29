(function(){
    //定时请求二维码状态
    var job = setInterval(getQrCodeStatus,2500);
    function getQrCodeStatus(){
        var requestId = $("#requestId").val();
        $.ajax({
            url: '/student/getQrCodeStatus?r=' + requestId,
            type: 'GET',
            success: function (result) {
                if (result.body.code == 0) {
                    //已扫描登录，进行跳转
                    window.location.href="/student/info";
                }
                if (result.body.code == 201){
                    //二维码失效，停止请求
                    clearInterval(job);
                }
            },
            error: function () {

            }
        });
    }

})();
