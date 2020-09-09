const merge = require("webpack-merge");
const path = require("path");

const common = require("./webpack.common.js");

module.exports = merge.smart(common, {
  mode: "development",
  devtool: "inline-source-map",
  devServer: {
    contentBase: path.resolve(__dirname, "dist"),
    historyApiFallback: true,
    host: "0.0.0.0",
    port: 4000,
    stats: "minimal",
  },
});
