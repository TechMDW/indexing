document.addEventListener("astilectron-ready", function () {
  // This will send a message to GO
  const searchBox = document.getElementById("search-bar");
  const searchBoxContainer = document.getElementById("search-box-container");

  searchBox.addEventListener("keyup", async (e) => {
    e.preventDefault(); // cancel the native event

    const searchString = e.target.value;

    // if search string is empty, clear the results
    if (searchString === "") {
      searchBoxContainer.style.borderBottomLeftRadius = "5px";
      searchBoxContainer.style.borderBottomRightRadius = "5px";
      document.getElementById("search-results").innerHTML = "";
      return;
    }

    searchBoxContainer.style.borderBottomLeftRadius = "0px";
    searchBoxContainer.style.borderBottomRightRadius = "0px";

    sendQuery(searchString);
  });

  astilectron.onMessage(function (message) {
    console.log(message);
    document.getElementById("search-results").innerHTML = "";

    // if there are no results, show a message
    if (!message) {
      const resultDiv = document.createElement("div");
      resultDiv.classList.add("item");

      const resultTitle = document.createElement("span");
      resultTitle.classList.add("title");
      resultTitle.innerText = "No results found";

      resultDiv.appendChild(resultTitle);

      document.getElementById("search-results").appendChild(resultDiv);
      return;
    }

    for (let i = 0; i < message.length; i++) {
      const result = message[i];

      const resultDiv = document.createElement("div");
      resultDiv.classList.add("item");

      const resultTitle = document.createElement("span");
      resultTitle.classList.add("title");
      resultTitle.innerText = result.name;

      const resultPath = document.createElement("span");
      resultPath.classList.add("path");
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
