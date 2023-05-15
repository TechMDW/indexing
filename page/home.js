document.addEventListener("astilectron-ready", function () {
  // This will send a message to GO
  const search = document.getElementById("search-bar");

  search.addEventListener("keyup", async (e) => {
    const searchString = e.target.value;

    sendQuery(searchString);
  });

  astilectron.onMessage(function (message) {
    console.log(message);
    document.getElementById("search-results").innerHTML = "";

    for (let i = 0; i < message.length; i++) {
      const result = message[i];

      const resultDiv = document.createElement("div");
      resultDiv.classList.add("result");

      const resultTitle = document.createElement("h3");
      resultTitle.innerText = result.name;

      const resultPath = document.createElement("p");
      resultPath.innerText = result.fullPath;

      resultDiv.appendChild(resultTitle);
      resultDiv.appendChild(resultPath);

      document.getElementById("search-results").appendChild(resultDiv);
    }
  });
});

function sendQuery(query) {
  astilectron.sendMessage(query, () => {});
}
