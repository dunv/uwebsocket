const HtmlWebpackPlugin = require("html-webpack-plugin");
const { CleanWebpackPlugin } = require("clean-webpack-plugin");
const path = require("path");

const config = {
  entry: {
    app: ["./app.tsx"],
  },
  resolve: {
    extensions: [".js", ".jsx", ".json", ".ts", ".tsx", ".less", ".css"],
  },
  module: {
    rules: [
      { test: /\.(ts|tsx)$/, loader: "ts-loader" },
      { test: /\.js$/, enforce: "pre", loader: "source-map-loader" },
    ],
  },
  plugins: [
    new HtmlWebpackPlugin({
      filename: "index.html",
      meta: {
        viewport:
          "width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no, shrink-to-fit=no",
        "apple-mobile-web-app-capable": "yes",
      },
      title: "uwebsocket test",
    }),
    new CleanWebpackPlugin(),
  ],
  mode: "development",
  devtool: "inline-source-map",
  devServer: {
    contentBase: path.resolve(__dirname, "dist"),
    historyApiFallback: true,
    host: "0.0.0.0",
    port: 4000,
    stats: "minimal",
  },
};

module.exports = config;
