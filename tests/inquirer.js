inquirer.prompt([{
    type: 'list',
    message: '选择启动整个项目还是单个项目',
    name: '项目文件',
    choices: ['整个项目', 'wap', 'h5']
}]).then(function (answers) {
    console.log(answers)
    console.log('ok')
})
