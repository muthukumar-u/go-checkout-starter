// Prices and totals always come from the API. The browser stores only product
// IDs, quantities, and the currently applied coupon. Refresh resets the demo.
const state = {
  products: [],
  items: [{ product_id: "notebook", quantity: 1 }, { product_id: "mug", quantity: 1 }, { product_id: "lamp", quantity: 1 }],
  coupon: "",
  quote: null,
  busy: false,
};

const byId = (id) => document.getElementById(id);
const money = (paise) => new Intl.NumberFormat("en-IN", { style: "currency", currency: "INR", minimumFractionDigits: 2 }).format(paise / 100);

// Build text with textContent, so server messages never become HTML.
function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

async function api(path, options = {}) {
  const response = await fetch(path, { ...options, signal: AbortSignal.timeout(12000) });
  const data = await response.json();
  if (!response.ok) {
    const error = new Error(data.error?.message || "We couldn’t update your bag. Please try again.");
    error.field = data.error?.field;
    throw error;
  }
  return data;
}

function couponMessage(message = "", isError = false) {
  byId("coupon-message").textContent = message;
  byId("coupon-message").classList.toggle("error", isError);
  byId("coupon-code").setAttribute("aria-invalid", String(isError));
}

function setBusy(busy) {
  state.busy = busy;
  byId("totals").setAttribute("aria-busy", String(busy));
  document.querySelectorAll(".quantity-control button, .remove-item, .add-product, .offer button").forEach((button) => {
    button.disabled = busy || button.dataset.atLimit === "true";
  });
  byId("apply-coupon").disabled = busy || state.items.length === 0;
  byId("remove-coupon").disabled = busy;
  byId("review-button").disabled = busy || !state.quote;
}

function renderCart() {
  const root = byId("cart-items");
  root.replaceChildren();
  const count = state.items.reduce((total, item) => total + item.quantity, 0);
  byId("item-count").textContent = `(${count})`;
  byId("header-count").textContent = count;

  if (!count) {
    const empty = element("div", "empty-cart");
    empty.append(element("strong", "", "A little room for good things."), element("p", "", "Add something from the collection below to get started."));
    root.append(empty);
  }

  state.items.forEach((item) => {
    const product = state.products.find((product) => product.id === item.product_id);
    const row = element("article", "cart-item");
    const info = element("div", "cart-product");
    const image = element("img");
    image.src = product.image;
    image.alt = product.name;
    image.width = 96;
    image.height = 112;
    const details = element("div");
    const remove = element("button", "remove-item", "Remove");
    remove.type = "button";
    remove.setAttribute("aria-label", `Remove ${product.name}`);
    remove.addEventListener("click", () => updateItem(product.id, 0));
    details.append(element("h3", "", product.name), element("p", "", product.description), remove);
    info.append(image, details);

    const quantity = element("div", "quantity-control");
    [-1, 1].forEach((change) => {
      const button = element("button", "", change === -1 ? "−" : "+");
      button.type = "button";
      button.setAttribute("aria-label", `${change === -1 ? "Decrease" : "Increase"} quantity of ${product.name}`);
      button.dataset.atLimit = String(change === 1 && item.quantity === 99);
      button.addEventListener("click", () => updateItem(product.id, item.quantity + change));
      quantity.append(button);
      if (change === -1) quantity.append(element("span", "", item.quantity));
    });
    const line = state.quote?.items.find((line) => line.product_id === product.id);
    row.append(info, quantity, element("span", "line-total", line ? money(line.line_total) : "—"));
    root.append(row);
  });

  document.querySelectorAll(".add-product").forEach((button) => {
    button.dataset.atLimit = String(state.items.some((item) => item.product_id === button.dataset.productId && item.quantity >= 99));
  });
}

function renderTotals() {
  byId("subtotal").textContent = state.quote ? money(state.quote.subtotal) : state.items.length ? "—" : money(0);
  byId("total").textContent = state.quote ? money(state.quote.total) : state.items.length ? "—" : money(0);
  byId("discount-row").hidden = !state.quote?.coupon_code;
  byId("discount-code").textContent = state.quote?.coupon_code || "";
  byId("discount").textContent = state.quote ? `−${money(state.quote.discount)}` : "";
  byId("remove-coupon").hidden = !state.coupon;
}

function requestQuote(coupon) {
  return api("/api/checkout", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ items: state.items, coupon_code: coupon }),
  });
}

// Disable cart actions while a request runs. This keeps an older response from
// overwriting a newer cart. Coupon validation errors can be corrected in place.
async function refreshQuote(candidate = state.coupon, cartChanged = false) {
  setBusy(true);
  byId("page-error").hidden = true;
  couponMessage();
  try {
    if (!state.items.length) {
      state.quote = null;
      state.coupon = "";
      byId("coupon-code").value = "";
      return true;
    }
    try {
      state.quote = await requestQuote(candidate);
      state.coupon = state.quote.coupon_code || "";
      byId("coupon-code").value = state.coupon;
      if (state.coupon) couponMessage(`${state.coupon} applied. A good thing just got better.`);
    } catch (error) {
      if (error.field !== "coupon_code") throw error;
      // Changing the cart can make an applied offer ineligible. Clear that
      // coupon and fetch a valid quote so no stale total is presented.
      if (cartChanged) {
        state.coupon = "";
        byId("coupon-code").value = "";
        state.quote = await requestQuote("");
      }
      couponMessage(error.message, true);
      return false;
    }
    return true;
  } catch (error) {
    state.quote = null;
    byId("page-error").textContent = "We couldn’t update your bag. Check your connection, then apply a coupon or review your order to retry.";
    byId("page-error").hidden = false;
    // Permit retrying a failed quote through the review button below.
    return false;
  } finally {
    renderCart();
    renderTotals();
    setBusy(false);
    byId("review-button").disabled = state.items.length === 0;
  }
}

async function updateItem(productId, quantity) {
  if (state.busy) return;
  const existing = state.items.find((item) => item.product_id === productId);
  if (quantity === 0) state.items = state.items.filter((item) => item.product_id !== productId);
  else if (existing) existing.quantity = quantity;
  else state.items.push({ product_id: productId, quantity });
  state.quote = null;
  renderCart();
  renderTotals();
  await refreshQuote(state.coupon, true);
}

function renderProducts() {
  byId("product-grid").replaceChildren();
  state.products.forEach((product) => {
    const card = element("article", "product-card");
    const imageBox = element("div", "product-image");
    const image = element("img");
    image.src = product.image;
    image.alt = product.name;
    image.width = 400;
    image.height = 340;
    const add = element("button", "add-product", "+");
    add.type = "button";
    add.dataset.productId = product.id;
    add.setAttribute("aria-label", `Add ${product.name} to bag`);
    add.addEventListener("click", () => {
      const current = state.items.find((item) => item.product_id === product.id);
      updateItem(product.id, (current?.quantity || 0) + 1);
    });
    imageBox.append(image, add);
    const details = element("div", "product-details");
    details.append(element("h3", "", product.name), element("span", "", money(product.price)));
    card.append(imageBox, details, element("p", "", product.description));
    byId("product-grid").append(card);
  });
}

function renderOffers(coupons) {
  coupons.forEach((coupon) => {
    const row = element("div", "offer");
    const button = element("button", "", coupon.code);
    button.type = "button";
    button.setAttribute("aria-label", `Apply ${coupon.code}: ${coupon.description}`);
    button.addEventListener("click", () => {
      if (state.busy || !state.items.length) return;
      byId("coupon-code").value = coupon.code;
      refreshQuote(coupon.code);
    });
    row.append(button, element("span", "", coupon.description));
    byId("coupon-offers").append(row);
  });
}

byId("coupon-form").addEventListener("submit", (event) => {
  event.preventDefault();
  if (state.busy || !state.items.length) return;
  const code = byId("coupon-code").value.trim();
  if (!code) {
    couponMessage("Enter a coupon code, or choose an offer below.", true);
    return;
  }
  refreshQuote(code);
});
byId("remove-coupon").addEventListener("click", () => refreshQuote(""));
byId("close-review").addEventListener("click", () => byId("review-dialog").close());
byId("back-to-bag").addEventListener("click", () => byId("review-dialog").close());
byId("review-button").addEventListener("click", async () => {
  if (state.busy || !state.items.length) return;
  // Revalidate with the server when reviewing, rather than trust a past quote.
  if (!await refreshQuote(state.coupon, true) || !state.quote) return;
  byId("review-items").replaceChildren();
  state.quote.items.forEach((item) => {
    const line = element("div", "review-line");
    line.append(element("span", "", `${item.name} × ${item.quantity}`), element("span", "", money(item.line_total)));
    byId("review-items").append(line);
  });
  byId("review-total").textContent = money(state.quote.total);
  byId("review-discount").textContent = state.quote.coupon_code ? `${state.quote.coupon_code} saved you ${money(state.quote.discount)}.` : "";
  byId("review-dialog").showModal();
});

async function start() {
  try {
    const [catalog, offers] = await Promise.all([api("/api/products"), api("/api/coupons")]);
    state.products = catalog.products;
    renderProducts();
    renderOffers(offers.coupons);
    await refreshQuote();
  } catch (error) {
    byId("cart-items").textContent = "Your bag will be here when you’re back online.";
    byId("page-error").textContent = "We couldn’t load the collection. Start the Go server and open the address printed in your terminal. Live Server can preview styles but does not provide the checkout API.";
    byId("page-error").hidden = false;
  }
}
start();
