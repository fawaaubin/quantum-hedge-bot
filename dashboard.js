// Chart.js et socket.io sont chargés globalement via <script> dans dashboard.html.

// Connexion SocketIO (même origine que l'API par défaut).
const API_BASE = window.location.origin.startsWith("http")
  ? window.location.origin
  : "http://localhost:8080";
const socket = io(API_BASE);

// Graphique des prix
const ctxPrice = document.getElementById("priceChart").getContext("2d");
const priceChart = new Chart(ctxPrice, {
  type: "line",
  data: {
    labels: [],
    datasets: [{
      label: "Prix",
      data: [],
      borderColor: "#0f0",
      backgroundColor: "rgba(0,255,0,0.1)",
      fill: true
    }]
  },
  options: { responsive: true }
});

// Graphique du capital
const ctxCapital = document.getElementById("capitalChart").getContext("2d");
const capitalChart = new Chart(ctxCapital, {
  type: "line",
  data: {
    labels: [],
    datasets: [{
      label: "Capital",
      data: [],
      borderColor: "#00f",
      backgroundColor: "rgba(0,0,255,0.1)",
      fill: true
    }]
  },
  options: { responsive: true }
});

// Graphique du drawdown
const ctxDD = document.getElementById("drawdownChart").getContext("2d");
const drawdownChart = new Chart(ctxDD, {
  type: "line",
  data: {
    labels: [],
    datasets: [{
      label: "Drawdown (%)",
      data: [],
      borderColor: "#f00",
      backgroundColor: "rgba(255,0,0,0.1)",
      fill: true
    }]
  },
  options: { responsive: true }
});

// Mise à jour temps réel via SocketIO
socket.on("price_update", data => {
  const now = new Date().toLocaleTimeString();

  // Prix
  priceChart.data.labels.push(now);
  priceChart.data.datasets[0].data.push(data.price);
  if (priceChart.data.labels.length > 50) {
    priceChart.data.labels.shift();
    priceChart.data.datasets[0].data.shift();
  }
  priceChart.update();

  // Signaux affichés en texte
  document.getElementById("signals").textContent = JSON.stringify(data.signals, null, 2);
});

// Mise à jour du capital et drawdown via API REST
async function updateStatus() {
  let json;
  try {
    const res = await fetch(`${API_BASE}/status`);
    json = await res.json();
  } catch (e) {
    return; // API momentanément indisponible : on réessaie au prochain tick.
  }
  const now = new Date().toLocaleTimeString();

  // KPIs
  document.getElementById("kpiCapital").textContent = Number(json.capital).toFixed(2);
  document.getElementById("kpiDD").textContent = (json.drawdown * 100).toFixed(2) + " %";
  document.getElementById("kpiPnl").textContent = Number(json.realized_pnl).toFixed(2);
  document.getElementById("kpiHalt").textContent = json.can_trade ? "actif" : "⛔ halte";

  capitalChart.data.labels.push(now);
  capitalChart.data.datasets[0].data.push(json.capital);
  if (capitalChart.data.labels.length > 50) {
    capitalChart.data.labels.shift();
    capitalChart.data.datasets[0].data.shift();
  }
  capitalChart.update();

  drawdownChart.data.labels.push(now);
  drawdownChart.data.datasets[0].data.push(json.drawdown * 100);
  if (drawdownChart.data.labels.length > 50) {
    drawdownChart.data.labels.shift();
    drawdownChart.data.datasets[0].data.shift();
  }
  drawdownChart.update();
}
setInterval(updateStatus, 5000);
